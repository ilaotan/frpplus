package basic

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo/v2"

	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
	"github.com/fatedier/frp/test/e2e/pkg/port"
)

var _ = ginkgo.Describe("[Feature: Basic]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.Describe("TCP", func() {
		types := []string{"tcp"}
		for _, t := range types {
			proxyType := t
			ginkgo.It(fmt.Sprintf("Expose a %s echo server", strings.ToUpper(proxyType)), func() {
				serverConf := consts.LegacyDefaultServerConfig
				var clientConf strings.Builder
				clientConf.WriteString(consts.LegacyDefaultClientConfig)

				localPortName := framework.TCPEchoServerPort
				protocol := "tcp"
				getProxyConf := func(proxyName string, portName string, extra string) string {
					return fmt.Sprintf(`
				[%s]
				type = %s
				local_port = {{ .%s }}
				remote_port = {{ .%s }}
				`+extra, proxyName, proxyType, localPortName, portName)
				}

				tests := []struct {
					proxyName   string
					portName    string
					extraConfig string
				}{
					{
						proxyName: "normal",
						portName:  port.GenName("Normal"),
					},
					{
						proxyName:   "with-encryption",
						portName:    port.GenName("WithEncryption"),
						extraConfig: "use_encryption = true",
					},
					{
						proxyName:   "with-compression",
						portName:    port.GenName("WithCompression"),
						extraConfig: "use_compression = true",
					},
					{
						proxyName: "with-encryption-and-compression",
						portName:  port.GenName("WithEncryptionAndCompression"),
						extraConfig: `
						use_encryption = true
						use_compression = true
						`,
					},
				}

				// build all client config
				for _, test := range tests {
					clientConf.WriteString(getProxyConf(test.proxyName, test.portName, test.extraConfig) + "\n")
				}
				// run frps and frpc
				f.RunProcesses(serverConf, []string{clientConf.String()})

				for _, test := range tests {
					framework.NewRequestExpect(f).
						Protocol(protocol).
						PortName(test.portName).
						Explain(test.proxyName).
						Ensure()
				}
			})
		}
	})

})
