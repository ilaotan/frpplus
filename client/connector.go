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

package client

import (
	"context"
	"crypto/tls"
	"net"
	"strconv"
	"sync"
	"time"

	fmux "github.com/fatedier/yamux"
	"github.com/samber/lo"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/proto/wire"
	"github.com/fatedier/frp/pkg/transport"
	netpkg "github.com/fatedier/frp/pkg/util/net"
	"github.com/fatedier/frp/pkg/util/xlog"
)

// Connector is an interface for establishing connections to the server.
type Connector interface {
	Open() error
	Connect() (net.Conn, error)
	Close() error
}

type MessageConnector interface {
	Connect() (*msg.Conn, error)
	Close() error
}

type messageConnector struct {
	connector    Connector
	wireProtocol string
}

func newMessageConnector(connector Connector, wireProtocol string) *messageConnector {
	return &messageConnector{
		connector:    connector,
		wireProtocol: wireProtocol,
	}
}

func (c *messageConnector) Connect() (*msg.Conn, error) {
	conn, err := c.connector.Connect()
	if err != nil {
		return nil, err
	}
	if err = wire.WriteMagicIfV2(conn, c.wireProtocol); err != nil {
		conn.Close()
		return nil, err
	}
	return msg.NewConn(conn, msg.NewReadWriter(conn, c.wireProtocol)), nil
}

func (c *messageConnector) Close() error {
	return c.connector.Close()
}

// defaultConnectorImpl is the default implementation of Connector for normal frpc.
type defaultConnectorImpl struct {
	ctx context.Context
	cfg *v1.ClientCommonConfig

	muxSession *fmux.Session
	closeOnce  sync.Once
}

func NewConnector(ctx context.Context, cfg *v1.ClientCommonConfig) Connector {
	return &defaultConnectorImpl{
		ctx: ctx,
		cfg: cfg,
	}
}

// Open opens an underlying connection to the server.
// If TCPMux isn't enabled, the underlying connection is nil, you will get a new real TCP connection every time you call Connect().
func (c *defaultConnectorImpl) Open() error {
	xl := xlog.FromContextSafe(c.ctx)

	if !lo.FromPtr(c.cfg.Transport.TCPMux) {
		return nil
	}

	conn, err := c.realConnect()
	if err != nil {
		return err
	}

	fmuxCfg := fmux.DefaultConfig()
	fmuxCfg.KeepAliveInterval = time.Duration(c.cfg.Transport.TCPMuxKeepaliveInterval) * time.Second
	// Use trace level for yamux logs
	fmuxCfg.LogOutput = xlog.NewTraceWriter(xl)
	fmuxCfg.MaxStreamWindowSize = 6 * 1024 * 1024
	session, err := fmux.Client(conn, fmuxCfg)
	if err != nil {
		conn.Close()
		return err
	}
	c.muxSession = session
	return nil
}

// Connect returns a stream from the underlying connection, or a new TCP connection if TCPMux isn't enabled.
func (c *defaultConnectorImpl) Connect() (net.Conn, error) {
	if c.muxSession != nil {
		stream, err := c.muxSession.OpenStream()
		if err != nil {
			return nil, err
		}
		return stream, nil
	}

	return c.realConnect()
}

func (c *defaultConnectorImpl) realConnect() (net.Conn, error) {
	xl := xlog.FromContextSafe(c.ctx)
	var tlsConfig *tls.Config
	var err error
	if lo.FromPtr(c.cfg.Transport.TLS.Enable) {
		sn := c.cfg.Transport.TLS.ServerName
		if sn == "" {
			sn = c.cfg.ServerAddr
		}

		tlsConfig, err = transport.NewClientTLSConfigWithSystemCA(
			c.cfg.Transport.TLS.CertFile,
			c.cfg.Transport.TLS.KeyFile,
			c.cfg.Transport.TLS.TrustedCaFile,
			sn,
			c.cfg.Transport.TLS.UseSystemCa)
		if err != nil {
			xl.Warnf("fail to build tls configuration, err: %v", err)
			return nil, err
		}
	}

	dialer := &net.Dialer{
		Timeout:   time.Duration(c.cfg.Transport.DialServerTimeout) * time.Second,
		KeepAlive: time.Duration(c.cfg.Transport.DialServerKeepAlive) * time.Second,
	}
	if c.cfg.Transport.ConnectServerLocalIP != "" {
		localAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(c.cfg.Transport.ConnectServerLocalIP, "0"))
		if err != nil {
			xl.Warnf("fail to parse connectServerLocalIP %q: %v", c.cfg.Transport.ConnectServerLocalIP, err)
		} else {
			dialer.LocalAddr = localAddr
		}
	}

	conn, err := dialer.DialContext(
		c.ctx,
		"tcp",
		net.JoinHostPort(c.cfg.ServerAddr, strconv.Itoa(c.cfg.ServerPort)),
	)
	if err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		return conn, nil
	}

	// frps tells a TLS handshake apart from a plain connection by its first
	// byte, so the custom byte has to be written before the handshake starts.
	if !lo.FromPtr(c.cfg.Transport.TLS.DisableCustomTLSFirstByte) {
		if _, err := conn.Write([]byte{byte(netpkg.FRPTLSHeadByte)}); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return tls.Client(conn, tlsConfig), nil
}

func (c *defaultConnectorImpl) Close() error {
	c.closeOnce.Do(func() {
		if c.muxSession != nil {
			_ = c.muxSession.Close()
		}
	})
	return nil
}
