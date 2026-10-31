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
	"time"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func newTestController(t *testing.T) *Controller {
	t.Helper()
	ctl, err := New(v1.IPAccessControlConfig{
		StorePath:      filepath.Join(t.TempDir(), "store.json"),
		DefaultMode:    v1.IPAccessModeAllowlist,
		RetentionHours: 24,
		MaxRecords:     100,
		SelfAllowHours: 1,
	})
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}
	t.Cleanup(func() { _ = ctl.Close() })
	return ctl
}

// A periodic self allow call refreshes the validity of the rule. The matcher
// works on the compiled index, which holds a copy of every rule, so the index
// must be rebuilt when the validity changes. Otherwise the rule keeps the
// expiry it had when it was created and stops matching once that one passed,
// even though the stored rule is still valid.
func TestAllowIPRefreshKeepsTheRuleMatching(t *testing.T) {
	ctl := newTestController(t)
	addr := netip.MustParseAddr("203.0.113.7")

	if _, err := ctl.AllowIP("203.0.113.7", 1); err != nil {
		t.Fatalf("add the rule: %v", err)
	}
	// The next call refreshes it to a longer validity.
	if _, err := ctl.AllowIP("203.0.113.7", 2); err != nil {
		t.Fatalf("refresh the rule: %v", err)
	}

	// One hour later the validity of the first call is over, the refreshed one
	// is not.
	now := time.Now().Unix() + 3601
	if _, ok := ctl.index.match(GlobalProxyName, addr, now); !ok {
		t.Fatal("the refreshed rule must still match after the original validity ended")
	}
}

// A permanent rule must not be turned into a temporary one by a periodic call.
func TestAllowIPKeepsPermanentRules(t *testing.T) {
	ctl := newTestController(t)

	rule, err := ctl.AddRule(GlobalProxyName, ActionAllow, "203.0.113.8", "manual")
	if err != nil {
		t.Fatalf("add the rule: %v", err)
	}
	if rule.ExpiresAt != 0 {
		t.Fatalf("a rule added by hand must be permanent, got %d", rule.ExpiresAt)
	}

	updated, err := ctl.AllowIP("203.0.113.8", 1)
	if err != nil {
		t.Fatalf("self allow: %v", err)
	}
	if updated.ExpiresAt != 0 {
		t.Fatalf("the permanent rule must keep its validity, got %d", updated.ExpiresAt)
	}
}
