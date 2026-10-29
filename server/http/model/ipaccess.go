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

package model

type V2IPAccessSettingsResp struct {
	Enabled     bool   `json:"enabled"`
	StorePath   string `json:"storePath"`
	GlobalMode  string `json:"globalMode"`
	RuleCount   int    `json:"ruleCount"`
	RecordCount int    `json:"recordCount"`
	// RetentionHours is how long an access record is kept after it was seen
	// for the last time.
	RetentionHours int64 `json:"retentionHours"`
	MaxRecords     int64 `json:"maxRecords"`
	// ProxyModes holds the proxies that override the global mode.
	ProxyModes []V2IPAccessProxyModeResp `json:"proxyModes"`
}

type V2IPAccessProxyModeResp struct {
	ProxyName string `json:"proxyName"`
	Mode      string `json:"mode"`
}

type V2IPAccessRuleResp struct {
	ID        uint64 `json:"id"`
	ProxyName string `json:"proxyName"`
	Action    string `json:"action"`
	IP        string `json:"ip"`
	Note      string `json:"note,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	// ExpiresAt is the unix time the rule stops being effective, 0 means the
	// rule is permanent.
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// Source is "manual" for rules created in the dashboard, "api" for rules
	// created by the self-allow api.
	Source string `json:"source,omitempty"`
}

// V2IPAccessSelfAllowResp is returned to the caller of the self-allow api, it
// only describes what happened to its own address.
type V2IPAccessSelfAllowResp struct {
	IP        string `json:"ip"`
	ProxyName string `json:"proxyName"`
	Action    string `json:"action"`
	RuleID    uint64 `json:"ruleId"`
	// ExpiresAt is 0 when the matched rule is permanent.
	ExpiresAt int64 `json:"expiresAt"`
}

type V2IPAccessRecordResp struct {
	ID        uint64 `json:"id"`
	ProxyName string `json:"proxyName"`
	IP        string `json:"ip"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	ConnCount int64  `json:"connCount"`
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason,omitempty"`
}

type V2IPAccessPruneResp struct {
	Cleared int `json:"cleared"`
	Total   int `json:"total"`
}

type V2IPAccessClearedResp struct {
	Cleared int `json:"cleared"`
}
