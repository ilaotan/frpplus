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
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// compiledRule is a Rule with its network parsed once, so that the per
// connection check only does a prefix lookup.
type compiledRule struct {
	Rule
	prefix netip.Prefix
}

// ruleIndex keeps rules grouped by proxy for fast lookup. Rules of a proxy are
// searched first and fall back to the global ones, so that a proxy can override
// the global policy for a specific address.
type ruleIndex struct {
	global  []compiledRule
	byProxy map[string][]compiledRule
}

func buildRuleIndex(rules []Rule) ruleIndex {
	idx := ruleIndex{byProxy: map[string][]compiledRule{}}
	for _, r := range rules {
		prefix, err := parseRuleIP(r.IP)
		if err != nil {
			continue
		}
		cr := compiledRule{Rule: r, prefix: prefix}
		if r.ProxyName == GlobalProxyName {
			idx.global = append(idx.global, cr)
			continue
		}
		idx.byProxy[r.ProxyName] = append(idx.byProxy[r.ProxyName], cr)
	}
	return idx
}

// match returns the most specific rule that covers addr. When two rules have
// the same prefix length, deny wins so that a block cannot be loosened by
// accident. Rules that already expired are ignored, the store keeps them until
// the next cleanup.
func (idx *ruleIndex) match(proxyName string, addr netip.Addr, now int64) (Rule, bool) {
	if r, ok := matchRules(idx.byProxy[proxyName], addr, now); ok {
		return r, true
	}
	return matchRules(idx.global, addr, now)
}

func matchRules(rules []compiledRule, addr netip.Addr, now int64) (Rule, bool) {
	var (
		best    Rule
		bestLen = -1
		found   bool
	)
	for _, r := range rules {
		if r.ExpiresAt > 0 && r.ExpiresAt <= now {
			continue
		}
		if !r.prefix.Contains(addr) {
			continue
		}
		if n := r.prefix.Bits(); n > bestLen || (n == bestLen && r.Action == ActionDeny) {
			best = r.Rule
			bestLen = n
			found = true
		}
	}
	return best, found
}

// parseRuleIP accepts a single address or a CIDR block and returns its
// canonical prefix.
func parseRuleIP(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, fmt.Errorf("ip is empty")
	}
	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("invalid cidr %q: %v", raw, err)
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid ip %q: %v", raw, err)
	}
	return netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()), nil
}

// canonicalIP normalizes an address so that "::ffff:1.2.3.4" and "1.2.3.4" are
// treated as the same client.
func canonicalIP(addr netip.Addr) string {
	return addr.Unmap().String()
}

// ClientIPFromRemoteAddr returns the canonical client IP of a net.Addr style
// string. It is exported for the dashboard API, which has to trust the address
// of the TCP connection and never a value sent by the caller.
func ClientIPFromRemoteAddr(remoteAddr string) (string, error) {
	addr, err := parseRemoteAddr(remoteAddr)
	if err != nil {
		return "", err
	}
	return canonicalIP(addr), nil
}

// IsLoopbackOrUnspecified reports whether the address cannot be the real client
// of a direct request, which usually means the request went through a local
// reverse proxy.
func IsLoopbackOrUnspecified(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.IsLoopback() || addr.IsUnspecified()
}

// normalizeRuleIP returns the canonical form of a rule target: a plain address
// stays a plain address, a CIDR block is masked.
func normalizeRuleIP(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "/") {
		prefix, err := parseRuleIP(raw)
		if err != nil {
			return "", err
		}
		return prefix.String(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return "", fmt.Errorf("invalid ip %q: %v", raw, err)
	}
	return addr.Unmap().String(), nil
}

// parseRemoteAddr extracts the client address from a net.Addr style string.
func parseRemoteAddr(remoteAddr string) (netip.Addr, error) {
	if addrPort, err := netip.ParseAddrPort(remoteAddr); err == nil {
		return addrPort.Addr().Unmap(), nil
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid remote address %q: %v", remoteAddr, err)
	}
	return addr.Unmap(), nil
}

// normalizeMode validates a mode and returns it in canonical form. An empty
// mode is only accepted by SetProxyMode, where it means "inherit".
func normalizeMode(mode string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	if slices.Contains(SupportedModes, m) {
		return m, nil
	}
	return "", fmt.Errorf("invalid mode %q, optional values are %v", mode, SupportedModes)
}

// normalizeAction validates a rule action.
func normalizeAction(action string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(action))
	if slices.Contains(SupportedActions, a) {
		return a, nil
	}
	return "", fmt.Errorf("invalid action %q, optional values are %v", action, SupportedActions)
}
