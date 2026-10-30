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
	v1 "github.com/fatedier/frp/pkg/config/v1"
)

const (
	// ActionAllow explicitly allows a client IP.
	ActionAllow = "allow"
	// ActionDeny explicitly denies a client IP.
	ActionDeny = "deny"

	// GlobalProxyName is the proxy name used by rules and modes that apply to
	// every tcp proxy.
	GlobalProxyName = "*"

	// SourceManual marks a rule created by the operator in the dashboard.
	SourceManual = "manual"
	// SourceAPI marks a rule created by the self-allow API, it always carries
	// an expiry.
	SourceAPI = "api"

	// snapshotVersion is written into the store file to allow future migrations.
	snapshotVersion = 1
)

// SupportedActions lists the valid values of Rule.Action.
var SupportedActions = []string{ActionAllow, ActionDeny}

// SupportedModes lists the valid values of a mode.
var SupportedModes = []string{
	v1.IPAccessModeOff,
	v1.IPAccessModeAllowlist,
	v1.IPAccessModeDenylist,
}

// Rule is a single allow or deny entry for a client IP or a CIDR block.
type Rule struct {
	ID uint64 `json:"id"`
	// ProxyName is the tcp proxy the rule belongs to, GlobalProxyName means
	// all proxies.
	ProxyName string `json:"proxyName"`
	// Action is either ActionAllow or ActionDeny.
	Action string `json:"action"`
	// IP is a single address or a CIDR block in its canonical form.
	IP        string `json:"ip"`
	Note      string `json:"note,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	// ExpiresAt is the unix time the rule stops being effective, 0 means the
	// rule is permanent.
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// Source is SourceManual or SourceAPI. An empty value means SourceManual,
	// so that rules written by older versions keep their meaning.
	Source string `json:"source,omitempty"`
}

// Record aggregates the accesses of one client IP on one proxy. Records are
// kept for operators to review traffic in the dashboard and to promote an
// address into a rule later. They live in memory only: they are expired by
// retention and by the record count limit, and a restart clears them.
type Record struct {
	ID        uint64 `json:"id"`
	ProxyName string `json:"proxyName"`
	IP        string `json:"ip"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	ConnCount int64  `json:"connCount"`
	// Allowed is the result of the last decision for this address.
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Decision is the result of checking a user connection.
type Decision struct {
	Allowed bool
	// Mode is the effective mode of the proxy.
	Mode string
	// Reason explains the decision, it is logged when a connection is denied.
	Reason string
}

// Settings is a read-only view of the controller configuration and statistics.
type Settings struct {
	Enabled        bool
	StorePath      string
	GlobalMode     string
	RetentionHours int64
	MaxRecords     int64
	RuleCount      int
	RecordCount    int
	// ProxyModes holds the proxies that override the global mode.
	ProxyModes map[string]string
}

// snapshot is the on-disk representation of the store.
// snapshot is what gets written to the store file. Access records are not part
// of it: they are only cached in memory.
type snapshot struct {
	Version    int               `json:"version"`
	NextID     uint64            `json:"nextID"`
	GlobalMode string            `json:"globalMode"`
	ProxyModes map[string]string `json:"proxyModes,omitempty"`
	Rules      []Rule            `json:"rules"`
}
