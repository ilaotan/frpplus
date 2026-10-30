package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxyCloneDeepCopy(t *testing.T) {
	require := require.New(t)

	enabled := true
	cfg := &TCPProxyConfig{
		ProxyBaseConfig: ProxyBaseConfig{
			Name:        "p1",
			Type:        "tcp",
			Enabled:     &enabled,
			Annotations: map[string]string{"a": "1"},
			Metadatas:   map[string]string{"m": "1"},
			HealthCheck: HealthCheckConfig{
				Type: "http",
				HTTPHeaders: []HTTPHeader{
					{Name: "X-Test", Value: "v1"},
				},
			},
		},
		RemotePort: 6000,
	}

	cloned := cfg.Clone().(*TCPProxyConfig)

	*cloned.Enabled = false
	cloned.Annotations["a"] = "changed"
	cloned.Metadatas["m"] = "changed"
	cloned.HealthCheck.HTTPHeaders[0].Value = "changed"

	require.True(*cfg.Enabled)
	require.Equal("1", cfg.Annotations["a"])
	require.Equal("1", cfg.Metadatas["m"])
	require.Equal("v1", cfg.HealthCheck.HTTPHeaders[0].Value)
	require.Equal(6000, cfg.RemotePort)
}
