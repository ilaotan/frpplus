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

package http

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/server/http/model"
	"github.com/fatedier/frp/server/ipaccess"
)

type ipAccessSettingsReq struct {
	GlobalMode string `json:"globalMode"`
}

type ipAccessProxyModeReq struct {
	ProxyName string `json:"proxyName"`
	Mode      string `json:"mode"`
}

type ipAccessRuleReq struct {
	ProxyName string `json:"proxyName"`
	Action    string `json:"action"`
	IP        string `json:"ip"`
	Note      string `json:"note"`
}

type ipAccessRecordsDeleteReq struct {
	IDs []uint64 `json:"ids"`
}

func (c *Controller) ipAccessController() (*ipaccess.Controller, error) {
	if c.ipAccessCtl == nil {
		return nil, httppkg.NewError(http.StatusBadRequest,
			"ip access control is not enabled, set ipAccessControl.enable = true in the frps configuration and restart")
	}
	return c.ipAccessCtl, nil
}

// /api/v2/ip-access/settings
func (c *Controller) APIV2IPAccessSettings(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	return buildIPAccessSettingsResp(ctl.Settings()), nil
}

// /api/v2/ip-access/settings
func (c *Controller) APIV2IPAccessUpdateSettings(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	var req ipAccessSettingsReq
	if err := ctx.BindJSON(&req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
	}
	if err := ctl.SetGlobalMode(req.GlobalMode); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	return buildIPAccessSettingsResp(ctl.Settings()), nil
}

// /api/v2/ip-access/proxy-modes
func (c *Controller) APIV2IPAccessUpdateProxyMode(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	var req ipAccessProxyModeReq
	if err := ctx.BindJSON(&req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
	}
	if err := ctl.SetProxyMode(req.ProxyName, req.Mode); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	return buildIPAccessSettingsResp(ctl.Settings()), nil
}

// /api/v2/ip-access/rules
func (c *Controller) APIV2IPAccessRuleList(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	page, pageSize, err := parseV2PageParams(ctx)
	if err != nil {
		return nil, err
	}
	action := strings.ToLower(strings.TrimSpace(ctx.Query("action")))
	if action != "" && !slices.Contains(ipaccess.SupportedActions, action) {
		return nil, httppkg.NewError(http.StatusBadRequest, "action must be one of allow, deny")
	}

	rules := ctl.ListRules(strings.TrimSpace(ctx.Query("proxyName")), action)
	items := make([]model.V2IPAccessRuleResp, 0, len(rules))
	for _, r := range rules {
		items = append(items, model.V2IPAccessRuleResp{
			ID:        r.ID,
			ProxyName: r.ProxyName,
			Action:    r.Action,
			IP:        r.IP,
			Note:      r.Note,
			CreatedAt: r.CreatedAt,
			ExpiresAt: r.ExpiresAt,
			Source:    r.Source,
		})
	}
	return buildV2PageResp(items, page, pageSize), nil
}

// /api/v2/ip-access/rules
func (c *Controller) APIV2IPAccessRuleCreate(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	var req ipAccessRuleReq
	if err := ctx.BindJSON(&req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
	}
	rule, err := ctl.AddRule(req.ProxyName, req.Action, req.IP, req.Note)
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	return model.V2IPAccessRuleResp{
		ID:        rule.ID,
		ProxyName: rule.ProxyName,
		Action:    rule.Action,
		IP:        rule.IP,
		Note:      rule.Note,
		CreatedAt: rule.CreatedAt,
		ExpiresAt: rule.ExpiresAt,
		Source:    rule.Source,
	}, nil
}

// /api/v2/ip-access/rules/{id}
func (c *Controller) APIV2IPAccessRuleDelete(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid rule id")
	}
	if err := ctl.DeleteRule(id); err != nil {
		return nil, httppkg.NewError(http.StatusNotFound, err.Error())
	}
	return nil, nil
}

// /api/v2/ip-access/records
func (c *Controller) APIV2IPAccessRecordList(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	page, pageSize, err := parseV2PageParams(ctx)
	if err != nil {
		return nil, err
	}
	allowed, err := parseIPAccessResultFilter(ctx.Query("result"))
	if err != nil {
		return nil, err
	}

	records := ctl.ListRecords(strings.TrimSpace(ctx.Query("proxyName")), ctx.Query("ip"), allowed)
	items := make([]model.V2IPAccessRecordResp, 0, len(records))
	for _, r := range records {
		items = append(items, model.V2IPAccessRecordResp{
			ID:        r.ID,
			ProxyName: r.ProxyName,
			IP:        r.IP,
			FirstSeen: r.FirstSeen,
			LastSeen:  r.LastSeen,
			ConnCount: r.ConnCount,
			Allowed:   r.Allowed,
			Reason:    r.Reason,
		})
	}
	return buildV2PageResp(items, page, pageSize), nil
}

// /api/v2/ip-access/records/delete
func (c *Controller) APIV2IPAccessRecordDelete(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	var req ipAccessRecordsDeleteReq
	if err := ctx.BindJSON(&req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
	}
	return model.V2IPAccessClearedResp{Cleared: ctl.DeleteRecords(req.IDs)}, nil
}

// /api/v2/ip-access/prune
func (c *Controller) APIV2IPAccessPrune(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	cleared := ctl.Prune()
	return model.V2IPAccessPruneResp{
		Cleared: cleared,
		Total:   ctl.Settings().RecordCount,
	}, nil
}

// selfAllowMaxHours caps the validity a caller may ask for, 30 days.
const selfAllowMaxHours = 24 * 30

// /api/v2/ip-access/self-allow
//
// Adds the client IP of the caller to the global allow list, so that a client
// whose public IP keeps changing can let itself in without touching the
// dashboard. The address is always read from the TCP connection: accepting an
// ip from the caller would turn this endpoint into an allow-anyone switch.
//
// It is protected by the dashboard basic auth middleware.
func (c *Controller) APIV2IPAccessSelfAllow(ctx *httppkg.Context) (any, error) {
	ctl, err := c.ipAccessController()
	if err != nil {
		return nil, err
	}
	cfg := c.serverCfg.IPAccessControl
	if !cfg.EnableSelfAllow {
		return nil, httppkg.NewError(http.StatusBadRequest,
			"self allow is not enabled, set ipAccessControl.enableSelfAllow = true in the frps configuration and restart")
	}
	// When the dashboard runs without credentials the basic auth middleware lets
	// every request through, which must not happen for this endpoint.
	if c.serverCfg.WebServer.User == "" || c.serverCfg.WebServer.Password == "" {
		return nil, httppkg.NewError(http.StatusForbidden,
			"self allow requires webServer.user and webServer.password to be set")
	}

	ip, err := ipaccess.ClientIPFromRemoteAddr(ctx.Req.RemoteAddr)
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	// A loopback address means the request came through a local reverse proxy.
	// Adding 127.0.0.1 would silently allow something useless, so fail loudly.
	if ipaccess.IsLoopbackOrUnspecified(ip) {
		return nil, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf(
			"the request comes from %s, which is not the address of the caller; "+
				"call this api directly instead of through a local reverse proxy", ip))
	}

	hours, err := parseSelfAllowHours(ctx, cfg.SelfAllowHours)
	if err != nil {
		return nil, err
	}

	rule, err := ctl.AllowIP(ip, hours)
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	log.Infof("ip access control: self allow for [%s], %d hour(s), expires at [%s]",
		ip, hours, formatOptionalUnix(rule.ExpiresAt))
	return model.V2IPAccessSelfAllowResp{
		IP:        rule.IP,
		ProxyName: rule.ProxyName,
		Action:    rule.Action,
		RuleID:    rule.ID,
		ExpiresAt: rule.ExpiresAt,
	}, nil
}

func parseSelfAllowHours(ctx *httppkg.Context, defaultHours int64) (int64, error) {
	raw := strings.TrimSpace(ctx.Query("hours"))
	if raw == "" {
		return defaultHours, nil
	}
	hours, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || hours <= 0 {
		return 0, httppkg.NewError(http.StatusBadRequest, "hours must be a positive integer")
	}
	if hours > selfAllowMaxHours {
		hours = selfAllowMaxHours
	}
	return hours, nil
}

func formatOptionalUnix(ts int64) string {
	if ts == 0 {
		return "never"
	}
	return time.Unix(ts, 0).Format(time.RFC3339)
}

func parseIPAccessResultFilter(raw string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return nil, nil
	case "allowed":
		v := true
		return &v, nil
	case "denied":
		v := false
		return &v, nil
	default:
		return nil, httppkg.NewError(http.StatusBadRequest, "result must be one of all, allowed, denied")
	}
}

func buildIPAccessSettingsResp(s ipaccess.Settings) model.V2IPAccessSettingsResp {
	proxyModes := make([]model.V2IPAccessProxyModeResp, 0, len(s.ProxyModes))
	for name, mode := range s.ProxyModes {
		proxyModes = append(proxyModes, model.V2IPAccessProxyModeResp{ProxyName: name, Mode: mode})
	}
	slices.SortFunc(proxyModes, func(a, b model.V2IPAccessProxyModeResp) int {
		return strings.Compare(a.ProxyName, b.ProxyName)
	})
	return model.V2IPAccessSettingsResp{
		Enabled:        s.Enabled,
		StorePath:      s.StorePath,
		GlobalMode:     s.GlobalMode,
		RuleCount:      s.RuleCount,
		RecordCount:    s.RecordCount,
		RetentionHours: s.RetentionHours,
		MaxRecords:     s.MaxRecords,
		ProxyModes:     proxyModes,
	}
}
