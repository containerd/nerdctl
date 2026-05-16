/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

// https://docs.docker.com/reference/cli/dockerd/#insecure-registries
// Local registries, whose IP address falls in the 127.0.0.0/8 range, are automatically marked as insecure as of Docker 1.3.2.
// It isn't recommended to rely on this, as it may change in the future.
// "--insecure" means that either the certificates are untrusted, or that the protocol is plain http
package login

import (
	"fmt"
	"net"
	"strconv"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/utils"
	"github.com/containerd/nerdctl/mod/tigron/utils/testca"

	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

// randomPort tells the registry helpers to acquire a free port automatically.
const randomPort = 0

type Client struct {
	args       []string
	configPath string
}

func (ag *Client) WithInsecure(value bool) *Client {
	ag.args = append(ag.args, "--insecure-registry="+strconv.FormatBool(value))
	return ag
}

func (ag *Client) WithHostsDir(hostDirs string) *Client {
	ag.args = append(ag.args, "--hosts-dir", hostDirs)
	return ag
}

func (ag *Client) WithCredentials(username, password string) *Client {
	if username != "" {
		ag.args = append(ag.args, "--username", username)
	}
	if password != "" {
		ag.args = append(ag.args, "--password", password)
	}
	return ag
}

func (ag *Client) WithConfigPath(value string) *Client {
	ag.configPath = value
	return ag
}

func (ag *Client) Cmd(helpers test.Helpers, host string) test.TestableCommand {
	if ag.configPath == "" {
		ag.configPath = helpers.T().TempDir()
	}
	args := []string{"login"}
	if !nerdtest.IsDocker() {
		args = append(args, "--debug-full")
	}
	args = append(args, ag.args...)
	args = append(args, host)
	cmd := helpers.Command(args...)
	cmd.Setenv("DOCKER_CONFIG", ag.configPath)
	return cmd
}

func TestLoginPersistence(t *testing.T) {
	nerdtest.Setup()

	var basicReg *registry.Server
	var tokenReg *registry.Server
	var tokenAS *registry.TokenAuthServer

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			nerdtest.Registry,
		),
		SubTests: []*test.Case{
			{
				Description: "basic",
				Setup: func(data test.Data, helpers test.Helpers) {
					username := utils.RandomStringBase64(30) + "∞"
					password := utils.RandomStringBase64(30) + ":∞"

					basicReg = nerdtest.RegistryWithBasicAuth(data, helpers, username, password, randomPort, false)
					basicReg.Setup(data, helpers)

					host := fmt.Sprintf("localhost:%d", basicReg.Port)
					configPath := helpers.T().TempDir()

					(&Client{configPath: configPath}).
						WithCredentials(username, password).
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})

					(&Client{configPath: configPath}).
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})

					(&Client{configPath: configPath}).
						WithCredentials("invalid", "invalid").
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{configPath: configPath}).
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if basicReg != nil {
						basicReg.Cleanup(data, helpers)
					}
				},
			},
			{
				Description: "token",
				Setup: func(data test.Data, helpers test.Helpers) {
					username := utils.RandomStringBase64(30) + "∞"
					password := utils.RandomStringBase64(30) + ":∞"

					// Use HTTP registry (nil CA) so localhost is trusted without explicit hosts-dir,
					// matching the original test behaviour. The auth server still uses a CA for JWT
					// signing even without TLS on the auth server itself.
					rca := testca.NewX509(data, helpers)
					tokenAS = registry.NewCesantaAuthServer(data, helpers, rca, randomPort, username, password, false)
					tokenAS.Setup(data, helpers)
					tokenReg = registry.NewDockerRegistry(data, helpers, nil, randomPort, tokenAS.Auth)
					tokenReg.Setup(data, helpers)

					host := fmt.Sprintf("localhost:%d", tokenReg.Port)
					configPath := helpers.T().TempDir()

					(&Client{configPath: configPath}).
						WithCredentials(username, password).
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})

					(&Client{configPath: configPath}).
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})

					(&Client{configPath: configPath}).
						WithCredentials("invalid", "invalid").
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{configPath: configPath}).
						Cmd(helpers, host).
						Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if tokenReg != nil {
						tokenReg.Cleanup(data, helpers)
					}
					if tokenAS != nil {
						tokenAS.Cleanup(data, helpers)
					}
				},
			},
		},
	}
	testCase.Run(t)
}

/*
func TestAgainstNoAuth(t *testing.T) {
	base := testutil.NewBase(t)
	t.Parallel()

	// Start the registry with the requested options
	reg := testregistry.NewRegistry(base, nil, randomPort, &testregistry.NoAuth{}, nil)

	// Register registry cleanup
	t.Cleanup(func() {
		reg.Cleanup(nil)
	})

	c := (&Client{}).
		WithCredentials("invalid", "invalid")

	c.Run(base, fmt.Sprintf("localhost:%d", reg.Port)).
		AssertOK()

	content, _ := os.ReadFile(filepath.Join(c.configPath, "config.json"))
	fmt.Println(string(content))

	c.Run(base, fmt.Sprintf("localhost:%d", reg.Port)).
		AssertFail()

}

*/

func TestLoginAgainstVariants(t *testing.T) {
	// Skip docker, because Docker doesn't have `--hosts-dir` nor `insecure-registry` option
	// This will test access to a wide variety of servers, with or without TLS, with basic or token authentication

	nerdtest.Setup()

	testCases := []struct {
		port int
		tls  bool
		auth string
	}{
		// Basic auth, no TLS
		{80, false, "basic"},
		{443, false, "basic"},
		{0, false, "basic"},
		// Token auth, no TLS
		{80, false, "token"},
		{443, false, "token"},
		{0, false, "token"},
		// Basic auth, with TLS
		/*
			// This is not working currently, unless we would force a server https:// in hosts
			// To be fixed with login rewrite
			{80, true, "basic"},
		*/
		{443, true, "basic"},
		{0, true, "basic"},
		// Token auth, with TLS
		/*
			// This is not working currently, unless we would force a server https:// in hosts
			// To be fixed with login rewrite
			{80, true, "token"},
		*/
		{443, true, "token"},
		{0, true, "token"},
	}

	var subtests []*test.Case
	for _, tc := range testCases {
		tc := tc

		var reg *registry.Server
		var tokenAuthServer *registry.TokenAuthServer

		subtests = append(subtests, &test.Case{
			Description: fmt.Sprintf("tls:%t port:%d auth:%s", tc.tls, tc.port, tc.auth),
			// Fixed-port cases must not run in parallel: children are parallelised,
			// and mixing Parallel levels can deadlock in Go's test runner.
			NoParallel: tc.port != 0,
			Setup: func(data test.Data, helpers test.Helpers) {
				username := utils.RandomStringBase64(30) + "∞"
				password := utils.RandomStringBase64(30) + ":∞"

				switch {
				case tc.auth == "basic":
					reg = nerdtest.RegistryWithBasicAuth(data, helpers, username, password, tc.port, tc.tls)
					reg.Setup(data, helpers)
				case tc.auth == "token" && tc.tls:
					reg, tokenAuthServer = nerdtest.RegistryWithTokenAuth(data, helpers, username, password, tc.port, tc.tls)
					tokenAuthServer.Setup(data, helpers)
					reg.Setup(data, helpers)
				default: // token auth, no TLS: HTTP registry + HTTP auth server (CA used only for JWT)
					rca := testca.NewX509(data, helpers)
					tokenAuthServer = registry.NewCesantaAuthServer(data, helpers, rca, randomPort, username, password, false)
					tokenAuthServer.Setup(data, helpers)
					reg = registry.NewDockerRegistry(data, helpers, nil, tc.port, tokenAuthServer.Auth)
					reg.Setup(data, helpers)
				}

				regHosts := []string{
					net.JoinHostPort(reg.IP.String(), strconv.Itoa(reg.Port)),
					net.JoinHostPort("localhost", strconv.Itoa(reg.Port)),
					net.JoinHostPort("127.0.0.1", strconv.Itoa(reg.Port)),
					// TODO: ipv6
				}
				if reg.Port == 443 {
					regHosts = append(regHosts,
						reg.IP.String(),
						"localhost",
						"127.0.0.1",
						// TODO: ipv6
					)
				}

				for _, regHost := range regHosts {
					rl, _ := dockerconfigresolver.Parse(regHost)

					// 1. valid credentials (no CA)
					// a. Insecure flag not being set
					// TODO: remove specialization when we fix the localhost mess
					if rl.IsLocalhost() && !tc.tls {
						(&Client{}).
							WithCredentials(username, password).
							Cmd(helpers, regHost).
							Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
					} else {
						(&Client{}).
							WithCredentials(username, password).
							Cmd(helpers, regHost).
							Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})
					}

					// b. Insecure flag set to false
					// TODO: remove specialization when we fix the localhost mess
					if !rl.IsLocalhost() {
						(&Client{}).
							WithCredentials(username, password).
							WithInsecure(false).
							Cmd(helpers, regHost).
							Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})
					}

					// c. Insecure flag set to true
					// TODO: remove specialization when we fix the localhost mess
					if !rl.IsLocalhost() || !tc.tls {
						(&Client{}).
							WithCredentials(username, password).
							WithInsecure(true).
							Cmd(helpers, regHost).
							Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
					}

					// 2. valid credentials (with access to server CA)
					{
						// a. Insecure flag not being set
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir)

						if tc.tls || rl.IsLocalhost() {
							c.Cmd(helpers, regHost).Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
						} else {
							c.Cmd(helpers, regHost).Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})
						}

						// b. Insecure flag set to false
						if tc.tls {
							c.WithInsecure(false).Cmd(helpers, regHost).Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
						} else {
							// TODO: remove specialization when we fix the localhost mess
							if !rl.IsLocalhost() {
								c.WithInsecure(false).Cmd(helpers, regHost).Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})
							}
						}

						// c. Insecure flag set to true
						c.WithInsecure(true).Cmd(helpers, regHost).Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
					}

					// 3. valid credentials, any url variant, should always succeed
					{
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir).
							// Just use insecure here for all servers - it does not matter for what we are testing here
							// in this case, which is whether we can successfully log in against any of these variants
							WithInsecure(true)

						// TODO: remove specialization when we fix the localhost mess
						if !rl.IsLocalhost() || !tc.tls {
							c.Cmd(helpers, "http://"+regHost).Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
							c.Cmd(helpers, "https://"+regHost).Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
							c.Cmd(helpers, "http://"+regHost+"/whatever?foo=bar;foo:bar#foo=bar").Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
							c.Cmd(helpers, "https://"+regHost+"/whatever?foo=bar&bar=foo;foo=foo+bar:bar#foo=bar").Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
						}
					}

					// 4. wrong password should always fail
					(&Client{}).
						WithCredentials(username, "invalid").
						WithHostsDir(reg.HostsDir).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials(username, "invalid").
						WithHostsDir(reg.HostsDir).
						WithInsecure(false).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials(username, "invalid").
						WithHostsDir(reg.HostsDir).
						WithInsecure(true).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials(username, "invalid").
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials(username, "invalid").
						WithInsecure(false).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials(username, "invalid").
						WithInsecure(true).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					// 5. wrong username should always fail
					(&Client{}).
						WithCredentials("invalid", password).
						WithHostsDir(reg.HostsDir).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials("invalid", password).
						WithHostsDir(reg.HostsDir).
						WithInsecure(false).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials("invalid", password).
						WithHostsDir(reg.HostsDir).
						WithInsecure(true).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials("invalid", password).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials("invalid", password).
						WithInsecure(false).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					(&Client{}).
						WithCredentials("invalid", password).
						WithInsecure(true).
						Cmd(helpers, regHost).
						Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if reg != nil {
					reg.Cleanup(data, helpers)
				}
				if tokenAuthServer != nil {
					tokenAuthServer.Cleanup(data, helpers)
				}
			},
		})
	}

	testCase := &test.Case{
		Require: require.All(
			require.Not(nerdtest.Docker),
			nerdtest.Registry,
		),
		SubTests: subtests,
	}
	testCase.Run(t)
}
