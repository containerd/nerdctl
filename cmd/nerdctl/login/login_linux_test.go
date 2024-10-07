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
	"os"
	"strconv"
	"testing"

	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

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

func (ag *Client) GetConfigPath() string {
	return ag.configPath
}

func (ag *Client) Run(base *testutil.Base, host string) *testutil.Cmd {
	if ag.configPath == "" {
		ag.configPath, _ = os.MkdirTemp(base.T.TempDir(), "docker-config")
	}
	args := []string{"login"}
	if base.Target == "nerdctl" {
		args = append(args, "--debug-full")
	}
	args = append(args, ag.args...)
	icmdCmd := icmd.Command(base.Binary, append(base.Args, append(args, host)...)...)
	icmdCmd.Env = append(base.Env, "HOME="+os.Getenv("HOME"), "DOCKER_CONFIG="+ag.configPath)

	return &testutil.Cmd{
		Cmd:  icmdCmd,
		Base: base,
	}
}

func TestLoginPersistence(t *testing.T) {
	base := testutil.NewBase(t)
	t.Parallel()

	// Retrieve from the store
	testCases := []struct {
		auth string
	}{
		{
			"basic",
		},
		{
			"token",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Server %s", tc.auth), func(t *testing.T) {
			t.Parallel()

			username := testregistry.SafeRandomString(30) + "∞"
			password := testregistry.SafeRandomString(30) + ":∞"

			// Add the requested authentication
			var auth testregistry.Auth
			var dependentCleanup func(error)

			auth = &testregistry.NoAuth{}
			if tc.auth == "basic" {
				auth = &testregistry.BasicAuth{
					Username: username,
					Password: password,
				}
			} else if tc.auth == "token" {
				authCa := testca.New(base.T)
				as := testregistry.NewAuthServer(base, authCa, 0, username, password, false)
				auth = &testregistry.TokenAuth{
					Address:  as.Scheme + "://" + net.JoinHostPort(as.IP.String(), strconv.Itoa(as.Port)),
					CertPath: as.CertPath,
				}
				dependentCleanup = as.Cleanup
			}

			// Start the registry with the requested options
			reg := testregistry.NewRegistry(base, nil, 0, auth, dependentCleanup)

			// Register registry cleanup
			t.Cleanup(func() {
				reg.Cleanup(nil)
			})

			// First, login successfully
			c := (&Client{}).
				WithCredentials(username, password)

			c.Run(base, fmt.Sprintf("localhost:%d", reg.Port)).
				AssertOK()

			// Now, log in successfully without passing any explicit credentials
			nc := (&Client{}).
				WithConfigPath(c.GetConfigPath())
			nc.Run(base, fmt.Sprintf("localhost:%d", reg.Port)).
				AssertOK()

			// Now fail while using invalid credentials
			nc.WithCredentials("invalid", "invalid").
				Run(base, fmt.Sprintf("localhost:%d", reg.Port)).
				AssertFail()

			// And login again without, reverting to the last saved good state
			nc = (&Client{}).
				WithConfigPath(c.GetConfigPath())

			nc.Run(base, fmt.Sprintf("localhost:%d", reg.Port)).
				AssertOK()
		})
	}
}

/*
func TestAgainstNoAuth(t *testing.T) {
	base := testutil.NewBase(t)
	t.Parallel()

	// Start the registry with the requested options
	reg := testregistry.NewRegistry(base, nil, 0, &testregistry.NoAuth{}, nil)

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
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	t.Parallel()

	testCases := []struct {
		port int
		tls  bool
		auth string
	}{
		// Basic auth, no TLS
		{
			80,
			false,
			"basic",
		},
		{
			443,
			false,
			"basic",
		},
		{
			0,
			false,
			"basic",
		},
		// Token auth, no TLS
		{
			80,
			false,
			"token",
		},
		{
			443,
			false,
			"token",
		},
		{
			0,
			false,
			"token",
		},
		// Basic auth, with TLS
		/*
			// This is not working currently, unless we would force a server https:// in hosts
			// To be fixed with login rewrite
			{
				80,
				true,
				"basic",
			},
		*/
		{
			443,
			true,
			"basic",
		},
		{
			0,
			true,
			"basic",
		},
		// Token auth, with TLS
		/*
			// This is not working currently, unless we would force a server https:// in hosts
			// To be fixed with login rewrite
			{
				80,
				true,
				"token",
			},
		*/
		{
			443,
			true,
			"token",
		},
		{
			0,
			true,
			"token",
		},
	}

	// Iterate through all cases, that will present a variety of port (80, 443, random), TLS (yes or no), and authentication (basic, token) type combinations
	for _, tc := range testCases {
		port := tc.port
		tls := tc.tls
		auth := tc.auth

		t.Run(fmt.Sprintf("Login against `tls: %t port: %d auth: %s`", tls, port, auth), func(t *testing.T) {
			// Tests with fixed ports should not be parallelized (although the port locking mechanism will prevent conflicts)
			// as their children tests are parallelized, and this might deadlock given the way `Parallel` works
			if port == 0 {
				t.Parallel()
			}

			// Generate credentials that are specific to each registry, so that we never cross hit another one
			username := testregistry.SafeRandomString(30) + "∞"
			password := testregistry.SafeRandomString(30) + ":∞"

			// Get a CA if we want TLS
			var ca *testca.CA
			if tls {
				ca = testca.New(base.T)
			}

			// Add the requested authenticator
			var authenticator testregistry.Auth
			var dependentCleanup func(error)

			authenticator = &testregistry.NoAuth{}
			if auth == "basic" {
				authenticator = &testregistry.BasicAuth{
					Username: username,
					Password: password,
				}
			} else if auth == "token" {
				authCa := ca
				// We could be on !tls, meaning no ca - but we still need a CA to sign jwt tokens
				if authCa == nil {
					authCa = testca.New(base.T)
				}
				as := testregistry.NewAuthServer(base, authCa, 0, username, password, tls)
				authenticator = &testregistry.TokenAuth{
					Address:  as.Scheme + "://" + net.JoinHostPort(as.IP.String(), strconv.Itoa(as.Port)),
					CertPath: as.CertPath,
				}
				dependentCleanup = as.Cleanup
			}

			// Start the registry with the requested options
			reg := testregistry.NewRegistry(base, ca, port, authenticator, dependentCleanup)

			// Register registry cleanup
			t.Cleanup(func() {
				reg.Cleanup(nil)
			})

			// Any registry is reachable through its ip+port, and localhost variants
			regHosts := []string{
				net.JoinHostPort(reg.IP.String(), strconv.Itoa(reg.Port)),
				net.JoinHostPort("localhost", strconv.Itoa(reg.Port)),
				net.JoinHostPort("127.0.0.1", strconv.Itoa(reg.Port)),
				// TODO: ipv6
				// net.JoinHostPort("::1", strconv.Itoa(reg.Port)),
			}

			// Registries that use port 443 also allow access without specifying a port
			if reg.Port == 443 {
				regHosts = append(regHosts, reg.IP.String())
				regHosts = append(regHosts, "localhost")
				regHosts = append(regHosts, "127.0.0.1")
				// TODO: ipv6
				// regHosts = append(regHosts, "::1")
			}

			// Iterate through these hosts access points, and create a test per-variant
			for _, value := range regHosts {
				regHost := value
				t.Run(regHost, func(t *testing.T) {
					t.Parallel()

					// 1. test with valid credentials but no access to the CA
					t.Run("1. valid credentials (no CA) ", func(t *testing.T) {
						t.Parallel()

						c := (&Client{}).
							WithCredentials(username, password)

						rl, _ := dockerconfigresolver.Parse(regHost)
						// a. Insecure flag not being set
						// TODO: remove specialization when we fix the localhost mess
						if rl.IsLocalhost() && !tls {
							c.Run(base, regHost).
								AssertOK()
						} else {
							c.Run(base, regHost).
								AssertFail()
						}

						// b. Insecure flag set to false
						// TODO: remove specialization when we fix the localhost mess
						if !rl.IsLocalhost() {
							(&Client{}).
								WithCredentials(username, password).
								WithInsecure(false).
								Run(base, regHost).
								AssertFail()
						}

						// c. Insecure flag set to true
						// TODO: remove specialization when we fix the localhost mess
						if !rl.IsLocalhost() || !tls {
							(&Client{}).
								WithCredentials(username, password).
								WithInsecure(true).
								Run(base, regHost).
								AssertOK()
						}
					})

					// 2. test with valid credentials AND access to the CA
					t.Run("2. valid credentials (with access to server CA)", func(t *testing.T) {
						t.Parallel()

						rl, _ := dockerconfigresolver.Parse(regHost)

						// a. Insecure flag not being set
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir)

						if tls || rl.IsLocalhost() {
							c.Run(base, regHost).
								AssertOK()
						} else {
							c.Run(base, regHost).
								AssertFail()
						}

						// b. Insecure flag set to false
						if tls {
							c.WithInsecure(false).
								Run(base, regHost).
								AssertOK()
						} else {
							// TODO: remove specialization when we fix the localhost mess
							if !rl.IsLocalhost() {
								c.WithInsecure(false).
									Run(base, regHost).
									AssertFail()
							}
						}

						// c. Insecure flag set to true
						c.WithInsecure(true).
							Run(base, regHost).
							AssertOK()
					})

					t.Run("3. valid credentials, any url variant, should always succeed", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir).
							// Just use insecure here for all servers - it does not matter for what we are testing here
							// in this case, which is whether we can successfully log in against any of these variants
							WithInsecure(true)

						// TODO: remove specialization when we fix the localhost mess
						rl, _ := dockerconfigresolver.Parse(regHost)
						if !rl.IsLocalhost() || !tls {
							c.Run(base, "http://"+regHost).AssertOK()
							c.Run(base, "https://"+regHost).AssertOK()
							c.Run(base, "http://"+regHost+"/whatever?foo=bar;foo:bar#foo=bar").AssertOK()
							c.Run(base, "https://"+regHost+"/whatever?foo=bar&bar=foo;foo=foo+bar:bar#foo=bar").AssertOK()
						}
					})

					t.Run("4. wrong password should always fail", func(t *testing.T) {
						t.Parallel()

						(&Client{}).
							WithCredentials(username, "invalid").
							WithHostsDir(reg.HostsDir).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials(username, "invalid").
							WithHostsDir(reg.HostsDir).
							WithInsecure(false).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials(username, "invalid").
							WithHostsDir(reg.HostsDir).
							WithInsecure(true).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials(username, "invalid").
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials(username, "invalid").
							WithInsecure(false).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials(username, "invalid").
							WithInsecure(true).
							Run(base, regHost).
							AssertFail()
					})

					t.Run("5. wrong username should always fail", func(t *testing.T) {
						t.Parallel()

						(&Client{}).
							WithCredentials("invalid", password).
							WithHostsDir(reg.HostsDir).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials("invalid", password).
							WithHostsDir(reg.HostsDir).
							WithInsecure(false).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials("invalid", password).
							WithHostsDir(reg.HostsDir).
							WithInsecure(true).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials("invalid", password).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials("invalid", password).
							WithInsecure(false).
							Run(base, regHost).
							AssertFail()

						(&Client{}).
							WithCredentials("invalid", password).
							WithInsecure(true).
							Run(base, regHost).
							AssertFail()
					})
				})
			}
		})
	}
}
