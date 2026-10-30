package client

import (
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
)

type failingConnector struct {
	err error
}

func (c *failingConnector) Open() error {
	return c.err
}

func (c *failingConnector) Connect() (net.Conn, error) {
	return nil, c.err
}

func (c *failingConnector) Close() error {
	return nil
}

func TestUpdateConfigSourceRollsBackReloadCommonOnReplaceAllFailure(t *testing.T) {
	prevCommon := &v1.ClientCommonConfig{User: "old-user"}
	newCommon := &v1.ClientCommonConfig{User: "new-user"}

	svr := &Service{
		configSource: source.NewConfigSource(),
		reloadCommon: prevCommon,
	}

	invalidProxy := &v1.TCPProxyConfig{}
	err := svr.UpdateConfigSource(newCommon, []v1.ProxyConfigurer{invalidProxy})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "proxy name cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}

	if svr.reloadCommon != prevCommon {
		t.Fatalf("reloadCommon should roll back on ReplaceAll failure")
	}
}

func TestUpdateConfigSourceKeepsReloadCommonOnReloadFailure(t *testing.T) {
	prevCommon := &v1.ClientCommonConfig{User: "old-user"}
	newCommon := &v1.ClientCommonConfig{User: "new-user"}

	svr := &Service{
		// Keep configSource valid so ReplaceAll succeeds first.
		configSource: source.NewConfigSource(),
		reloadCommon: prevCommon,
		// Keep aggregator nil to force reload failure.
		aggregator: nil,
	}

	validProxy := &v1.TCPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name: "p1",
			Type: "tcp",
		},
	}
	err := svr.UpdateConfigSource(newCommon, []v1.ProxyConfigurer{validProxy})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "config aggregator is not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}

	if svr.reloadCommon != newCommon {
		t.Fatalf("reloadCommon should keep new value on reload failure")
	}
}

func TestReloadConfigFromSourcesDoesNotMutateStoreConfigs(t *testing.T) {
	storeSource, err := source.NewStoreSource(source.StoreSourceConfig{
		Path: filepath.Join(t.TempDir(), "store.json"),
	})
	if err != nil {
		t.Fatalf("new store source: %v", err)
	}

	proxyCfg := &v1.TCPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name: "store-proxy",
			Type: "tcp",
		},
	}
	if err := storeSource.AddProxy(proxyCfg); err != nil {
		t.Fatalf("add proxy to store: %v", err)
	}

	agg := source.NewAggregator(source.NewConfigSource())
	agg.SetStoreSource(storeSource)
	svr := &Service{
		aggregator:   agg,
		configSource: agg.ConfigSource(),
		storeSource:  storeSource,
		reloadCommon: &v1.ClientCommonConfig{},
	}

	if err := svr.reloadConfigFromSources(); err != nil {
		t.Fatalf("reload config from sources: %v", err)
	}

	gotProxy := storeSource.GetProxy("store-proxy")
	if gotProxy == nil {
		t.Fatalf("proxy not found in store")
	}
	if gotProxy.GetBaseConfig().LocalIP != "" {
		t.Fatalf("store proxy localIP should stay empty, got %q", gotProxy.GetBaseConfig().LocalIP)
	}

	svr.cfgMu.RLock()
	defer svr.cfgMu.RUnlock()

	if len(svr.proxyCfgs) != 1 {
		t.Fatalf("expected 1 runtime proxy, got %d", len(svr.proxyCfgs))
	}
	if svr.proxyCfgs[0].GetBaseConfig().LocalIP != "127.0.0.1" {
		t.Fatalf("runtime proxy localIP should be defaulted, got %q", svr.proxyCfgs[0].GetBaseConfig().LocalIP)
	}
}
