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
	"net/netip"
	"path/filepath"
	"testing"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func newTestController(t *testing.T) *Controller {
	t.Helper()
	c, err := New(v1.IPAccessControlConfig{
		Enable:         true,
		StorePath:      filepath.Join(t.TempDir(), "store.json"),
		DefaultMode:    v1.IPAccessModeAllowlist,
		RetentionHours: 1,
		MaxRecords:     100,
	})
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// TestAllowIPRefreshKeepsIndexInSync covers the case of a periodic self-allow
// call: the rule keeps the same id but its validity is pushed forward. The
// index used by the per connection check must follow, otherwise the rule stops
// matching once the validity that was current when the index was built has
// passed.
func TestAllowIPRefreshKeepsIndexInSync(t *testing.T) {
	c := newTestController(t)
	addr := netip.MustParseAddr("1.2.3.4")

	first, err := c.AllowIP(addr.String(), 1)
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}

	// A later call refreshes the validity of the same rule.
	second, err := c.AllowIP(addr.String(), 2)
	if err != nil {
		t.Fatalf("refresh allow: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same rule id, got %d then %d", first.ID, second.ID)
	}
	if second.ExpiresAt <= first.ExpiresAt {
		t.Fatalf("expected the validity to be pushed forward, %d then %d", first.ExpiresAt, second.ExpiresAt)
	}

	// The index must hold the refreshed validity, otherwise the rule stops
	// matching at the expiry that was current when the index was built.
	c.ruleMu.RLock()
	indexed := c.index.global
	c.ruleMu.RUnlock()
	if len(indexed) != 1 {
		t.Fatalf("expected one indexed rule, got %d", len(indexed))
	}
	if indexed[0].ExpiresAt != second.ExpiresAt {
		t.Fatalf("indexed expiry is %d, want %d", indexed[0].ExpiresAt, second.ExpiresAt)
	}

	// A connection arriving after the first expiry but before the refreshed one
	// must still be allowed.
	rule, matched := c.index.match(GlobalProxyName, addr, first.ExpiresAt+1)
	if !matched {
		t.Fatal("the refreshed rule does not match after the first expiry")
	}
	if rule.Action != ActionAllow {
		t.Fatalf("expected an allow rule, got %s", rule.Action)
	}
}

// TestAllowIPRefreshKeepsRuleUsable covers the same refresh path through the
// public check: the rule must keep working while it is being refreshed.
func TestAllowIPRefreshKeepsRuleUsable(t *testing.T) {
	c := newTestController(t)
	addr := netip.MustParseAddr("1.2.3.4")

	if _, err := c.AllowIP(addr.String(), 1); err != nil {
		t.Fatalf("first allow: %v", err)
	}
	// A refresh with a shorter validity still has to be reflected by the index.
	if _, err := c.AllowIP(addr.String(), 1); err != nil {
		t.Fatalf("refresh allow: %v", err)
	}

	decision := c.Check("tcp", "test", "1.2.3.4:54321")
	if !decision.Allowed {
		t.Fatalf("expected the connection to be allowed, got %q", decision.Reason)
	}
}

// TestAllowIPKeepsPermanentRule makes sure a periodic call does not turn a
// permanent rule into a temporary one.
func TestAllowIPKeepsPermanentRule(t *testing.T) {
	c := newTestController(t)
	addr := netip.MustParseAddr("1.2.3.4")

	permanent, err := c.AllowIP(addr.String(), 0)
	if err != nil {
		t.Fatalf("permanent allow: %v", err)
	}
	if permanent.ExpiresAt != 0 {
		t.Fatalf("expected a permanent rule, got expiry %d", permanent.ExpiresAt)
	}

	refreshed, err := c.AllowIP(addr.String(), 1)
	if err != nil {
		t.Fatalf("refresh allow: %v", err)
	}
	if refreshed.ExpiresAt != 0 {
		t.Fatalf("the permanent rule was turned into a temporary one, expiry %d", refreshed.ExpiresAt)
	}
}

// TestAllowIPUpdatesExistingRule makes sure a refresh does not create a second
// rule for the same address.
func TestAllowIPUpdatesExistingRule(t *testing.T) {
	c := newTestController(t)
	addr := netip.MustParseAddr("1.2.3.4")

	if _, err := c.AllowIP(addr.String(), 1); err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if _, err := c.AllowIP(addr.String(), 1); err != nil {
		t.Fatalf("refresh allow: %v", err)
	}

	c.ruleMu.RLock()
	rules := len(c.rules)
	c.ruleMu.RUnlock()
	if rules != 1 {
		t.Fatalf("expected one rule, got %d", rules)
	}
}

// TestPruneRemovesExpiredRules covers the cleanup of the rules that are not
// refreshed anymore.
func TestPruneRemovesExpiredRules(t *testing.T) {
	c := newTestController(t)
	addr := netip.MustParseAddr("1.2.3.4")

	if _, err := c.AllowIP(addr.String(), 1); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// Expire the rule by hand, the prune uses the same clock as the check.
	c.ruleMu.Lock()
	c.rules[0].ExpiresAt = 1
	c.index = buildRuleIndex(c.rules)
	c.ruleMu.Unlock()

	c.Prune()

	c.ruleMu.RLock()
	rules := len(c.rules)
	indexed := len(c.index.global)
	c.ruleMu.RUnlock()
	if rules != 0 || indexed != 0 {
		t.Fatalf("expected the expired rule to be gone, %d rules and %d indexed left", rules, indexed)
	}
}
