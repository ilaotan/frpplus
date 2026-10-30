// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ipaccess

import (
	"cmp"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/util/log"
)

// Controller decides whether a user connection to a tcp proxy is allowed, based
// on the client IP seen by the server. It also records the client addresses so
// that operators can review them in the dashboard.
//
// Only tcp proxies are covered: their user connections reach the server, so the
// server is the only place where the real client IP is still available.
type Controller struct {
	cfg  v1.IPAccessControlConfig
	path string

	ruleMu     sync.RWMutex
	globalMode string
	proxyModes map[string]string
	rules      []Rule
	index      ruleIndex

	// records is a memory only cache of what was seen recently, it is never
	// written to the store: only rules and modes are worth persisting.
	recordMu sync.Mutex
	records  map[string]*Record

	// types holds the proxy types that are subject to the check, built once
	// from the configuration.
	types map[string]struct{}

	nextID atomic.Uint64

	stop      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// New creates a controller and loads the store file. The store is created on
// the first save, so a missing file is not an error.
func New(cfg v1.IPAccessControlConfig) (*Controller, error) {
	mode, err := normalizeMode(cfg.DefaultMode)
	if err != nil {
		return nil, fmt.Errorf("ipAccessControl.defaultMode: %v", err)
	}
	if cfg.RetentionHours <= 0 {
		return nil, fmt.Errorf("ipAccessControl.retentionHours must be positive")
	}
	if cfg.MaxRecords <= 0 {
		return nil, fmt.Errorf("ipAccessControl.maxRecords must be positive")
	}

	c := &Controller{
		cfg:        cfg,
		path:       cfg.StorePath,
		globalMode: mode,
		proxyModes: map[string]string{},
		records:    map[string]*Record{},
		types:      map[string]struct{}{},
		stop:       make(chan struct{}),
	}
	// An empty list is treated as tcp only, the same as the configuration
	// default.
	if len(cfg.Types) == 0 {
		c.types[string(v1.ProxyTypeTCP)] = struct{}{}
	}
	for _, t := range cfg.Types {
		c.types[t] = struct{}{}
	}
	if err := c.load(); err != nil {
		return nil, err
	}

	c.wg.Add(1)
	go c.cleanupLoop()
	return c, nil
}

// Types returns the proxy types that are subject to the check, in the order
// used by the configuration so that the dashboard always reads the same.
func (c *Controller) Types() []string {
	order := []string{
		string(v1.ProxyTypeTCP),
		string(v1.ProxyTypeHTTP),
		string(v1.ProxyTypeHTTPS),
		string(v1.ProxyTypeTCPMUX),
	}
	types := make([]string, 0, len(c.types))
	for _, t := range order {
		if _, ok := c.types[t]; ok {
			types = append(types, t)
		}
	}
	return types
}

// Check decides whether a user connection should be allowed. Every connection
// of a covered proxy type is recorded, whatever the decision is.
func (c *Controller) Check(proxyType, proxyName, remoteAddr string) Decision {
	if _, ok := c.types[proxyType]; !ok {
		return Decision{Allowed: true, Reason: "ip access control does not apply to this proxy type"}
	}
	addr, err := parseRemoteAddr(remoteAddr)
	if err != nil {
		// An unparsable address is a server side problem, do not block on it.
		log.Warnf("ip access control: %v, the connection is allowed", err)
		return Decision{Allowed: true, Reason: "unparsable client address"}
	}
	decision := c.evaluate(proxyName, addr)
	c.record(proxyName, canonicalIP(addr), decision)
	return decision
}

func (c *Controller) evaluate(proxyName string, addr netip.Addr) Decision {
	c.ruleMu.RLock()
	mode := c.globalMode
	if m, ok := c.proxyModes[proxyName]; ok && m != "" {
		mode = m
	}
	index := c.index
	c.ruleMu.RUnlock()

	if mode == v1.IPAccessModeOff {
		return Decision{Allowed: true, Mode: mode, Reason: "mode is off"}
	}

	rule, matched := index.match(proxyName, addr, time.Now().Unix())
	if matched {
		return Decision{
			Allowed: rule.Action == ActionAllow,
			Mode:    mode,
			Reason:  fmt.Sprintf("matched %s rule #%d for %s", rule.Action, rule.ID, rule.IP),
		}
	}
	if mode == v1.IPAccessModeAllowlist {
		return Decision{Allowed: false, Mode: mode, Reason: "no allow rule matched"}
	}
	return Decision{Allowed: true, Mode: mode, Reason: "no deny rule matched"}
}

// record aggregates one connection into the access records.
func (c *Controller) record(proxyName, ip string, decision Decision) {
	now := time.Now().Unix()
	key := recordKey(proxyName, ip)

	c.recordMu.Lock()
	r, ok := c.records[key]
	if ok {
		r.LastSeen = now
		r.ConnCount++
		r.Allowed = decision.Allowed
		r.Reason = decision.Reason
	} else {
		c.records[key] = &Record{
			ID:        c.nextID.Add(1),
			ProxyName: proxyName,
			IP:        ip,
			FirstSeen: now,
			LastSeen:  now,
			ConnCount: 1,
			Allowed:   decision.Allowed,
			Reason:    decision.Reason,
		}
	}
	if len(c.records) > int(c.cfg.MaxRecords) {
		if removed := c.pruneLocked(now); removed > 0 {
			log.Debugf("ip access control: dropped %d access records over the limit, %d left", removed, len(c.records))
		}
	}
	c.recordMu.Unlock()
}

// Prune removes the expired rules and access records, and returns the number of
// removed records. Only the rule part touches the store file.
func (c *Controller) Prune() int {
	now := time.Now().Unix()
	c.pruneExpiredRules(now)

	c.recordMu.Lock()
	defer c.recordMu.Unlock()
	return c.pruneLocked(now)
}

// pruneExpiredRules drops the rules added by the self-allow API once they are no
// longer valid. Permanent rules are kept.
func (c *Controller) pruneExpiredRules(now int64) {
	c.ruleMu.Lock()
	before := len(c.rules)
	c.rules = slices.DeleteFunc(c.rules, func(r Rule) bool {
		return r.ExpiresAt > 0 && r.ExpiresAt <= now
	})
	if len(c.rules) == before {
		c.ruleMu.Unlock()
		return
	}
	c.index = buildRuleIndex(c.rules)
	c.ruleMu.Unlock()

	if err := c.save(); err != nil {
		log.Warnf("ip access control: %v", err)
		return
	}
	log.Infof("ip access control: removed %d expired rules", before-len(c.rules))
}

// pruneLocked must be called with recordMu held.
func (c *Controller) pruneLocked(now int64) int {
	expireBefore := now - c.cfg.RetentionHours*3600
	removed := 0
	for key, r := range c.records {
		if r.LastSeen < expireBefore {
			delete(c.records, key)
			removed++
		}
	}

	maxRecords := int(c.cfg.MaxRecords)
	if len(c.records) <= maxRecords {
		return removed
	}
	items := make([]*Record, 0, len(c.records))
	for _, r := range c.records {
		items = append(items, r)
	}
	slices.SortFunc(items, func(a, b *Record) int {
		return cmp.Compare(a.LastSeen, b.LastSeen)
	})
	// Trim to 90% of the limit so that the sort does not run again on every new
	// connection once the limit has been reached.
	target := maxRecords * 9 / 10
	for i := 0; i < len(items)-target; i++ {
		delete(c.records, recordKey(items[i].ProxyName, items[i].IP))
		removed++
	}
	return removed
}

func (c *Controller) Settings() Settings {
	c.ruleMu.RLock()
	s := Settings{
		Enabled:        c.cfg.Enable,
		StorePath:      c.path,
		GlobalMode:     c.globalMode,
		RetentionHours: c.cfg.RetentionHours,
		MaxRecords:     c.cfg.MaxRecords,
		RuleCount:      len(c.rules),
		ProxyModes:     map[string]string{},
	}
	maps.Copy(s.ProxyModes, c.proxyModes)
	s.Types = c.Types()
	c.ruleMu.RUnlock()

	c.recordMu.Lock()
	s.RecordCount = len(c.records)
	c.recordMu.Unlock()
	return s
}

func (c *Controller) SetGlobalMode(mode string) error {
	m, err := normalizeMode(mode)
	if err != nil {
		return err
	}
	c.ruleMu.Lock()
	if c.globalMode == m {
		c.ruleMu.Unlock()
		return nil
	}
	c.globalMode = m
	c.ruleMu.Unlock()
	return c.save()
}

// SetProxyMode overrides the global mode for one proxy. An empty mode removes
// the override.
func (c *Controller) SetProxyMode(proxyName, mode string) error {
	proxyName = strings.TrimSpace(proxyName)
	if proxyName == "" || proxyName == GlobalProxyName {
		return fmt.Errorf("invalid proxy name %q, the global mode is set separately", proxyName)
	}

	c.ruleMu.Lock()
	if mode == "" {
		delete(c.proxyModes, proxyName)
		c.ruleMu.Unlock()
		return c.save()
	}
	m, err := normalizeMode(mode)
	if err != nil {
		c.ruleMu.Unlock()
		return err
	}
	c.proxyModes[proxyName] = m
	c.ruleMu.Unlock()
	return c.save()
}

func (c *Controller) ListRules(proxyName, action string) []Rule {
	c.ruleMu.RLock()
	items := make([]Rule, 0, len(c.rules))
	for _, r := range c.rules {
		if proxyName != "" && r.ProxyName != proxyName {
			continue
		}
		if action != "" && r.Action != action {
			continue
		}
		items = append(items, r)
	}
	c.ruleMu.RUnlock()

	slices.SortFunc(items, func(a, b Rule) int {
		return cmp.Compare(b.ID, a.ID)
	})
	return items
}

func (c *Controller) AddRule(proxyName, action, ip, note string) (Rule, error) {
	a, err := normalizeAction(action)
	if err != nil {
		return Rule{}, err
	}
	proxyName = strings.TrimSpace(proxyName)
	if proxyName == "" {
		proxyName = GlobalProxyName
	}
	normalizedIP, err := normalizeRuleIP(ip)
	if err != nil {
		return Rule{}, err
	}
	prefix, err := parseRuleIP(normalizedIP)
	if err != nil {
		return Rule{}, err
	}

	c.ruleMu.Lock()
	for _, r := range c.rules {
		if r.ProxyName != proxyName || r.Action != a {
			continue
		}
		// Compare the parsed prefixes, so that 10.9.9.9 and 10.9.9.9/32 are
		// the same rule.
		existing, err := parseRuleIP(r.IP)
		if err == nil && existing == prefix {
			c.ruleMu.Unlock()
			return Rule{}, fmt.Errorf("the same rule already exists (id %d)", r.ID)
		}
	}
	rule := Rule{
		ID:        c.nextID.Add(1),
		ProxyName: proxyName,
		Action:    a,
		IP:        normalizedIP,
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now().Unix(),
		Source:    SourceManual,
	}
	c.rules = append(c.rules, rule)
	c.index = buildRuleIndex(c.rules)
	c.ruleMu.Unlock()

	if err := c.save(); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// AllowIP adds the client IP to the global allow list for the given number of
// hours. It is used by the self-allow API.
//
// The operation is idempotent, because the caller is expected to submit the same
// address periodically: the whole find-or-create runs under the rule lock, so
// concurrent calls cannot create two rules, an existing allow rule is extended
// in place instead of duplicated, and a rule that is already valid long enough
// is left untouched.
//
// hours 0 keeps the rule permanent. A permanent rule is never shortened.
func (c *Controller) AllowIP(ip string, hours int64) (Rule, error) {
	normalizedIP, err := normalizeRuleIP(ip)
	if err != nil {
		return Rule{}, err
	}
	prefix, err := parseRuleIP(normalizedIP)
	if err != nil {
		return Rule{}, err
	}

	now := time.Now().Unix()
	var expiresAt int64
	if hours > 0 {
		expiresAt = now + hours*3600
	}

	c.ruleMu.Lock()
	for i := range c.rules {
		r := &c.rules[i]
		if r.ProxyName != GlobalProxyName || r.Action != ActionAllow {
			continue
		}
		existing, err := parseRuleIP(r.IP)
		if err != nil || existing != prefix {
			continue
		}
		// Already permanent, or already valid long enough: nothing to do.
		if r.ExpiresAt == 0 || (expiresAt != 0 && r.ExpiresAt >= expiresAt) {
			rule := *r
			c.ruleMu.Unlock()
			return rule, nil
		}
		r.ExpiresAt = expiresAt
		r.Source = SourceAPI
		rule := *r
		c.ruleMu.Unlock()
		// The store only holds rules, so a full write here is a few hundred
		// bytes. Keep it synchronous: an allow rule must survive a crash.
		if err := c.save(); err != nil {
			return Rule{}, err
		}
		return rule, nil
	}

	rule := Rule{
		ID:        c.nextID.Add(1),
		ProxyName: GlobalProxyName,
		Action:    ActionAllow,
		IP:        normalizedIP,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Source:    SourceAPI,
	}
	c.rules = append(c.rules, rule)
	c.index = buildRuleIndex(c.rules)
	c.ruleMu.Unlock()

	if err := c.save(); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func (c *Controller) DeleteRule(id uint64) error {
	c.ruleMu.Lock()
	idx := slices.IndexFunc(c.rules, func(r Rule) bool { return r.ID == id })
	if idx < 0 {
		c.ruleMu.Unlock()
		return fmt.Errorf("rule %d not found", id)
	}
	c.rules = slices.Delete(c.rules, idx, idx+1)
	c.index = buildRuleIndex(c.rules)
	c.ruleMu.Unlock()
	return c.save()
}

// ListRecords returns the access records matching the filters, the most recent
// one first.
func (c *Controller) ListRecords(proxyName, ip string, allowed *bool) []Record {
	proxyName = strings.TrimSpace(proxyName)
	ip = strings.TrimSpace(ip)

	c.recordMu.Lock()
	items := make([]Record, 0, len(c.records))
	for _, r := range c.records {
		if proxyName != "" && r.ProxyName != proxyName {
			continue
		}
		if ip != "" && !strings.Contains(r.IP, ip) {
			continue
		}
		if allowed != nil && r.Allowed != *allowed {
			continue
		}
		items = append(items, *r)
	}
	c.recordMu.Unlock()

	slices.SortFunc(items, func(a, b Record) int {
		if v := cmp.Compare(b.LastSeen, a.LastSeen); v != 0 {
			return v
		}
		return cmp.Compare(a.IP, b.IP)
	})
	return items
}

func (c *Controller) DeleteRecords(ids []uint64) int {
	if len(ids) == 0 {
		return 0
	}
	want := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}

	c.recordMu.Lock()
	removed := 0
	for key, r := range c.records {
		if _, ok := want[r.ID]; ok {
			delete(c.records, key)
			removed++
		}
	}
	c.recordMu.Unlock()
	return removed
}

func recordKey(proxyName, ip string) string {
	return proxyName + "\x00" + ip
}
