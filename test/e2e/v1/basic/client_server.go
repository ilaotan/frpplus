package basic

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
	"github.com/fatedier/frp/test/e2e/pkg/cert"
	"github.com/fatedier/frp/test/e2e/pkg/port"
)

type generalTestConfigures struct {
	server        string
	client        string
	clientPrefix  string
	client2       string
	client2Prefix string
	testDelay     time.Duration
	expectError   bool
}

func runClientServerTest(f *framework.Framework, configures *generalTestConfigures) {
	serverConf := consts.DefaultServerConfig
	clientConf := consts.DefaultClientConfig
	if configures.clientPrefix != "" {
		clientConf = configures.clientPrefix
	}

	serverConf += fmt.Sprintf(`
	%s
	`, configures.server)

	tcpPortName := port.GenName("TCP")
	clientConf += fmt.Sprintf(`
		%s

		[[proxies]]
		name = "tcp"
		type = "tcp"
		localPort = {{ .%s }}
		remotePort = {{ .%s }}
		`, configures.client,
		framework.TCPEchoServerPort, tcpPortName,
	)

	clientConfs := []string{clientConf}
	if configures.client2 != "" {
		client2Conf := consts.DefaultClientConfig
		if configures.client2Prefix != "" {
			client2Conf = configures.client2Prefix
		}
		client2Conf += fmt.Sprintf(`
			%s
		`, configures.client2)
		clientConfs = append(clientConfs, client2Conf)
	}

	f.RunProcesses(serverConf, clientConfs)

	if configures.testDelay > 0 {
		time.Sleep(configures.testDelay)
	}

	framework.NewRequestExpect(f).PortName(tcpPortName).ExpectError(configures.expectError).Explain("tcp proxy").Ensure()
}

// defineClientServerTest test a normal tcp proxy with specified TestConfigures.
func defineClientServerTest(desc string, f *framework.Framework, configures *generalTestConfigures) {
	ginkgo.It(desc, func() {
		runClientServerTest(f, configures)
	})
}

var _ = ginkgo.Describe("[Feature: Client-Server]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.Describe("Protocol", func() {
		defineClientServerTest("tcp", f, &generalTestConfigures{})
	})

	ginkgo.Describe("Authentication", func() {
		defineClientServerTest("Token Correct", f, &generalTestConfigures{
			server: `auth.token = "123456"`,
			client: `auth.token = "123456"`,
		})

		defineClientServerTest("Token Incorrect", f, &generalTestConfigures{
			server:      `auth.token = "123456"`,
			client:      `auth.token = "invalid"`,
			expectError: true,
		})
	})

	ginkgo.Describe("TLS", func() {
		// Since v0.50.0, the default value of tls_enable has been changed to true.
		// Therefore, here it needs to be set as false to test the scenario of turning it off.
		defineClientServerTest("Disable TLS", f, &generalTestConfigures{
			client: `transport.tls.enable = false`,
		})

		defineClientServerTest("enable tls force, client with TLS", f, &generalTestConfigures{
			server: "transport.tls.force = true",
		})
		defineClientServerTest("enable tls force, client without TLS", f, &generalTestConfigures{
			server:      "transport.tls.force = true",
			client:      "transport.tls.enable = false",
			expectError: true,
		})
		defineClientServerTest("useSystemCa rejects the default self-signed server certificate", f, &generalTestConfigures{
			client:      "transport.tls.useSystemCa = true",
			expectError: true,
		})
	})

	ginkgo.Describe("TLS with custom certificate", func() {
		var (
			caCrtPath                    string
			serverCrtPath, serverKeyPath string
			clientCrtPath, clientKeyPath string
		)
		ginkgo.JustBeforeEach(func() {
			generator := &cert.SelfSignedCertGenerator{}
			artifacts, err := generator.Generate("127.0.0.1")
			framework.ExpectNoError(err)

			caCrtPath = f.WriteTempFile("ca.crt", string(artifacts.CACert))
			serverCrtPath = f.WriteTempFile("server.crt", string(artifacts.Cert))
			serverKeyPath = f.WriteTempFile("server.key", string(artifacts.Key))
			generator.SetCA(artifacts.CACert, artifacts.CAKey)
			_, err = generator.Generate("127.0.0.1")
			framework.ExpectNoError(err)
			clientCrtPath = f.WriteTempFile("client.crt", string(artifacts.Cert))
			clientKeyPath = f.WriteTempFile("client.key", string(artifacts.Key))
		})

		ginkgo.It("one-way authentication", func() {
			runClientServerTest(f, &generalTestConfigures{
				server: fmt.Sprintf(`
					transport.tls.trustedCaFile = "%s"
				`, caCrtPath),
				client: fmt.Sprintf(`
					transport.tls.certFile = "%s"
					transport.tls.keyFile = "%s"
				`, clientCrtPath, clientKeyPath),
			})
		})

		ginkgo.It("mutual authentication", func() {
			runClientServerTest(f, &generalTestConfigures{
				server: fmt.Sprintf(`
					transport.tls.certFile = "%s"
					transport.tls.keyFile = "%s"
					transport.tls.trustedCaFile = "%s"
				`, serverCrtPath, serverKeyPath, caCrtPath),
				client: fmt.Sprintf(`
					transport.tls.certFile = "%s"
					transport.tls.keyFile = "%s"
					transport.tls.trustedCaFile = "%s"
				`, clientCrtPath, clientKeyPath, caCrtPath),
			})
		})
	})

	ginkgo.Describe("TLS with custom certificate and specified server name", func() {
		var (
			caCrtPath                    string
			serverCrtPath, serverKeyPath string
			clientCrtPath, clientKeyPath string
		)
		ginkgo.JustBeforeEach(func() {
			generator := &cert.SelfSignedCertGenerator{}
			artifacts, err := generator.Generate("example.com")
			framework.ExpectNoError(err)

			caCrtPath = f.WriteTempFile("ca.crt", string(artifacts.CACert))
			serverCrtPath = f.WriteTempFile("server.crt", string(artifacts.Cert))
			serverKeyPath = f.WriteTempFile("server.key", string(artifacts.Key))
			generator.SetCA(artifacts.CACert, artifacts.CAKey)
			_, err = generator.Generate("example.com")
			framework.ExpectNoError(err)
			clientCrtPath = f.WriteTempFile("client.crt", string(artifacts.Cert))
			clientKeyPath = f.WriteTempFile("client.key", string(artifacts.Key))
		})

		ginkgo.It("mutual authentication", func() {
			runClientServerTest(f, &generalTestConfigures{
				server: fmt.Sprintf(`
				transport.tls.certFile = "%s"
				transport.tls.keyFile = "%s"
				transport.tls.trustedCaFile = "%s"
				`, serverCrtPath, serverKeyPath, caCrtPath),
				client: fmt.Sprintf(`
				transport.tls.serverName = "example.com"
				transport.tls.certFile = "%s"
				transport.tls.keyFile = "%s"
				transport.tls.trustedCaFile = "%s"
				`, clientCrtPath, clientKeyPath, caCrtPath),
			})
		})

		ginkgo.It("mutual authentication with incorrect server name", func() {
			runClientServerTest(f, &generalTestConfigures{
				server: fmt.Sprintf(`
				transport.tls.certFile = "%s"
				transport.tls.keyFile = "%s"
				transport.tls.trustedCaFile = "%s"
				`, serverCrtPath, serverKeyPath, caCrtPath),
				client: fmt.Sprintf(`
				transport.tls.serverName = "invalid.com"
				transport.tls.certFile = "%s"
				transport.tls.keyFile = "%s"
				transport.tls.trustedCaFile = "%s"
				`, clientCrtPath, clientKeyPath, caCrtPath),
				expectError: true,
			})
		})
	})

	ginkgo.Describe("TLS with disableCustomTLSFirstByte set to false", func() {
		defineClientServerTest("TLS", f, &generalTestConfigures{
			client: `
				transport.tls.disableCustomTLSFirstByte = false
			`,
		})
	})

	ginkgo.Describe("IPv6 bind address", func() {
		defineClientServerTest("IPv6 bind address", f, &generalTestConfigures{
			server: `
				bindAddr = "::"
			`,
		})
	})
})
