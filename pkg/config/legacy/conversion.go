// Copyright 2023 The frp Authors
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

package legacy

import (
	"github.com/fatedier/frp/pkg/config/types"
	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func Convert_ClientCommonConf_To_v1(conf *ClientCommonConf) *v1.ClientCommonConfig {
	out := &v1.ClientCommonConfig{}
	out.User = conf.User
	out.Auth.Method = v1.AuthMethod(conf.AuthenticationMethod)
	out.Auth.Token = conf.Token
	if conf.AuthenticateHeartBeats {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeHeartBeats)
	}
	if conf.AuthenticateNewWorkConns {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeNewWorkConns)
	}
	out.ServerAddr = conf.ServerAddr
	out.ServerPort = conf.ServerPort
	out.Transport.DialServerTimeout = conf.DialServerTimeout
	out.Transport.DialServerKeepAlive = conf.DialServerKeepAlive
	out.Transport.ConnectServerLocalIP = conf.ConnectServerLocalIP
	out.Transport.PoolCount = conf.PoolCount
	out.Transport.TCPMux = new(conf.TCPMux)
	out.Transport.TCPMuxKeepaliveInterval = conf.TCPMuxKeepaliveInterval
	out.Transport.Protocol = conf.Protocol
	out.Transport.HeartbeatInterval = conf.HeartbeatInterval
	out.Transport.HeartbeatTimeout = conf.HeartbeatTimeout
	out.Transport.TLS.Enable = new(conf.TLSEnable)
	out.Transport.TLS.DisableCustomTLSFirstByte = new(conf.DisableCustomTLSFirstByte)
	out.Transport.TLS.CertFile = conf.TLSCertFile
	out.Transport.TLS.KeyFile = conf.TLSKeyFile
	out.Transport.TLS.TrustedCaFile = conf.TLSTrustedCaFile
	out.Transport.TLS.ServerName = conf.TLSServerName

	out.Log.To = conf.LogFile
	out.Log.Level = conf.LogLevel
	out.Log.MaxDays = conf.LogMaxDays
	out.Log.DisablePrintColor = conf.DisableLogColor

	out.DNSServer = conf.DNSServer
	out.LoginFailExit = new(conf.LoginFailExit)
	out.Start = conf.Start
	out.Metadatas = conf.Metas
	out.IncludeConfigFiles = conf.IncludeConfigFiles
	return out
}

func Convert_ServerCommonConf_To_v1(conf *ServerCommonConf) *v1.ServerConfig {
	out := &v1.ServerConfig{}
	out.Auth.Method = v1.AuthMethod(conf.AuthenticationMethod)
	out.Auth.Token = conf.Token
	if conf.AuthenticateHeartBeats {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeHeartBeats)
	}
	if conf.AuthenticateNewWorkConns {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeNewWorkConns)
	}
	out.BindAddr = conf.BindAddr
	out.BindPort = conf.BindPort

	out.ProxyBindAddr = conf.ProxyBindAddr

	out.WebServer.Addr = conf.DashboardAddr
	out.WebServer.Port = conf.DashboardPort
	out.WebServer.User = conf.DashboardUser
	out.WebServer.Password = conf.DashboardPwd
	out.WebServer.AssetsDir = conf.AssetsDir
	if conf.DashboardTLSMode {
		out.WebServer.TLS = &v1.TLSConfig{}
		out.WebServer.TLS.CertFile = conf.DashboardTLSCertFile
		out.WebServer.TLS.KeyFile = conf.DashboardTLSKeyFile
		out.WebServer.PprofEnable = conf.PprofEnable
	}

	out.Log.To = conf.LogFile
	out.Log.Level = conf.LogLevel
	out.Log.MaxDays = conf.LogMaxDays
	out.Log.DisablePrintColor = conf.DisableLogColor

	out.DetailedErrorsToClient = new(conf.DetailedErrorsToClient)
	out.UserConnTimeout = conf.UserConnTimeout

	out.Transport.TCPMux = new(conf.TCPMux)
	out.Transport.TCPMuxKeepaliveInterval = conf.TCPMuxKeepaliveInterval
	out.Transport.TCPKeepAlive = conf.TCPKeepAlive
	out.Transport.MaxPoolCount = conf.MaxPoolCount
	out.Transport.HeartbeatTimeout = conf.HeartbeatTimeout

	out.Transport.TLS.Force = conf.TLSOnly
	out.Transport.TLS.CertFile = conf.TLSCertFile
	out.Transport.TLS.KeyFile = conf.TLSKeyFile
	out.Transport.TLS.TrustedCaFile = conf.TLSTrustedCaFile

	out.MaxPortsPerClient = conf.MaxPortsPerClient

	out.AllowPorts, _ = types.NewPortsRangeSliceFromString(conf.AllowPortsStr)
	return out
}

func Convert_ProxyConf_To_v1_Base(conf ProxyConf) *v1.ProxyBaseConfig {
	out := &v1.ProxyBaseConfig{}
	base := conf.GetBaseConfig()

	out.Name = base.ProxyName
	out.Type = base.ProxyType
	out.Metadatas = base.Metas

	out.Transport.UseEncryption = base.UseEncryption
	out.Transport.UseCompression = base.UseCompression
	out.Transport.BandwidthLimit = base.BandwidthLimit
	out.Transport.BandwidthLimitMode = base.BandwidthLimitMode
	out.Transport.ProxyProtocolVersion = base.ProxyProtocolVersion

	out.LoadBalancer.Group = base.Group
	out.LoadBalancer.GroupKey = base.GroupKey

	out.HealthCheck.Type = base.HealthCheckType
	out.HealthCheck.TimeoutSeconds = base.HealthCheckTimeoutS
	out.HealthCheck.MaxFailed = base.HealthCheckMaxFailed
	out.HealthCheck.IntervalSeconds = base.HealthCheckIntervalS
	out.HealthCheck.Path = base.HealthCheckURL

	out.LocalIP = base.LocalIP
	out.LocalPort = base.LocalPort
	return out
}

func Convert_ProxyConf_To_v1(conf ProxyConf) v1.ProxyConfigurer {
	outBase := Convert_ProxyConf_To_v1_Base(conf)
	var out v1.ProxyConfigurer
	switch v := conf.(type) {
	case *TCPProxyConf:
		c := &v1.TCPProxyConfig{ProxyBaseConfig: *outBase}
		c.RemotePort = v.RemotePort
		out = c
	}
	return out
}
