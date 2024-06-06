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

package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/cmd/login"
	"github.com/containerd/nerdctl/v2/pkg/dockerutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
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
	args := append([]string{"login", "--debug-full"}, ag.args...)
	icmdCmd := icmd.Command(base.Binary, append(base.Args, append(args, host)...)...)
	icmdCmd.Env = append(base.Env, "HOME="+os.Getenv("HOME"), "DOCKER_CONFIG="+ag.configPath)
	return &testutil.Cmd{
		Cmd:  icmdCmd,
		Base: base,
	}
}

func Match(thing string) func(stdout string) {
	return func(stdout string) {
		strings.Contains(stdout, thing)
	}
}

type TestCase struct {
	Description string
	Exp         *Expected
	Data        Meta

	TearUp             func(tID string)
	TearDown           func(tID string)
	Command            func(tID string) *testutil.Cmd
	Expected           func(tID string) icmd.Expected
	Inspect            func(t *testing.T, stdout string, stderr string)
	DockerIncompatible bool

	SubTests []*TestCase
}

func TestBrokenServers(t *testing.T) {
	base := testutil.NewBase(t)
	t.Parallel()

	testCases := []*TestCase{
		// TODO: failing to reach the DNS resolver
		/*
			{
				Description: "willneverresolve.whatever: ErrConnectionFailed (DNS resolution fail)",
				Command: func(tID string) *testutil.Cmd {
					return (&Client{}).
						WithCredentials("bla", "foo").
						Run(base, fmt.Sprintf("%s:%d", "willneverresolve.whatever", 0))
				},
				Expected: func(tID string) icmd.Expected {
					return icmd.Expected{
						ExitCode: 1,
						Out:      "",
						Err:      login.ErrConnectionFailed.Error(),
					}
				},
			},
			{
				Description: "ghcr.io:12345: ErrConnectionFailed (tcp timeout)",
				Command: func(tID string) *testutil.Cmd {
					return (&Client{}).
						WithCredentials("bla", "foo").
						Run(base, fmt.Sprintf("%s:%d", "ghcr.io", 12345))
				},
				Expected: func(tID string) icmd.Expected {
					return icmd.Expected{
						ExitCode: 1,
						Out:      "",
						Err:      login.ErrConnectionFailed.Error(),
					}
				},
			},
			{
				Description: "ghcr.io:80: ErrConnectionFailed (ErrSchemeMismatch)",
				Command: func(tID string) *testutil.Cmd {
					return (&Client{}).
						WithCredentials("bla", "foo").
						Run(base, fmt.Sprintf("%s:%d", "ghcr.io", 80))
				},
				Expected: func(tID string) icmd.Expected {
					return icmd.Expected{
						ExitCode: 1,
						Out:      "",
						Err:      login.ErrConnectionFailed.Error(),
					}
				},
				Inspect: func(t *testing.T, stdout string, stderr string) {
					assert.Assert(t,
						strings.Contains(stderr, http.ErrSchemeMismatch.Error()),
						"stderr should have matched http.ErrSchemeMismatch.Error()")
				},
			},
		*/

		//
		{
			Description: "140.82.116.34:443: invalid TLS certs",
			Command: func(tID string) *testutil.Cmd {
				return (&Client{}).
					WithCredentials("bla", "foo").
					Run(base, fmt.Sprintf("%s:%d", "140.82.116.34", 443))
			},
			Exp: &Expected{
				ExitCode: 1,
				Errors: []error{
					login.ErrConnectionFailed,
					http.ErrSchemeMismatch,
				},
				Output: func(stdout string) {
				},
			},
			Expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Error:    login.ErrConnectionFailed.Error(),
					Out:      "foo",
					Err:      "", // login.ErrConnectionFailed.Error(),
				}
			},
			Inspect: func(t *testing.T, stdout string, stderr string) {
				assert.Assert(t,
					strings.Contains(stderr, http.ErrSchemeMismatch.Error()),
					"stderr should have matched http.ErrSchemeMismatch.Error()")
			},
		},
	}

	for _, tc := range testCases {
		currentTest := tc
		t.Run(currentTest.Description, func(tt *testing.T) {
			if currentTest.DockerIncompatible {
				testutil.DockerIncompatible(tt)
			}

			tt.Parallel()

			tID := testutil.Identifier(tt)

			if currentTest.TearDown != nil {
				currentTest.TearDown(tID)
				tt.Cleanup(func() {
					currentTest.TearDown(tID)
				})
			}
			if currentTest.TearUp != nil {
				currentTest.TearUp(tID)
			}

			res := currentTest.Command(tID).Run()
			// assert.Assert(t, res.Compare(currentTest.Expected(tID))) //().Success()
			assert.Assert(t, res.Equal(currentTest.Expected(tID))) //().Success()

			// res.Assert(t, currentTest.Expected(tID))
			/*
				res.Assert(t, currentTest.Expected(tID))
				if currentTest.Inspect != nil {
					currentTest.Inspect(tt, res.Stdout(), res.Stderr())
				}

			*/
		})
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

			t.Run("Login repeats", func(t *testing.T) {
				t.Parallel()

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
		})
	}
}

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
		// Basic auth, with TLS
		{
			80,
			true,
			"basic",
		},
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
		// Token auth, with TLS
		{
			80,
			true,
			"token",
		},
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

	for _, tc := range testCases {
		port := tc.port
		tls := tc.tls
		auth := tc.auth

		// Iterate through all cases, that will present a variety of port (80, 443, random), TLS (yes or no), and authentication (basic, token) type combinations
		t.Run(fmt.Sprintf("Login against `tls: %t port: %d auth: %s`", tls, port, auth), func(t *testing.T) {
			// Tests with fixed ports should not be parallelized (although the port locking mechanism will prevent conflicts)
			// as their children tests are, and this might deadlock given the way `Parallel` works
			if port == 0 {
				t.Parallel()
			}

			// Generate credentials that are specific to a single registry, so that we never cross hit another one
			// Note that the grammar for basic auth does not allow colons in usernames, while token auth would allow it
			username := testregistry.SafeRandomString(30) + "∞"
			password := testregistry.SafeRandomString(30) + ":∞"

			// Get a CA if we want TLS
			var ca *testca.CA
			if tls {
				ca = testca.New(base.T)
			}

			// Add the requested authentication
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
				// We could be on !tls, meaning no ca - but we still need one to sign jwt tokens
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
				// net.JoinHostPort("::1", strconv.Itoa(reg.Port)),
			}

			// Registries that use port 443 also allow access without specifying a port
			if reg.Port == 443 {
				regHosts = append(regHosts, reg.IP.String())
				regHosts = append(regHosts, "localhost")
				regHosts = append(regHosts, "127.0.0.1")
				// regHosts = append(regHosts, "::1")
			}

			// Iterate through these hosts access points
			for _, value := range regHosts {
				regHost := value
				t.Run(regHost, func(t *testing.T) {
					t.Parallel()

					t.Run("Valid credentials (no CA) ", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, password)

						// Insecure flag not being set, localhost defaults to `insecure=true` and will succeed
						rl, _ := dockerutil.Parse(regHost)
						if rl.IsLocalhost() {
							c.Run(base, regHost).
								AssertOK()
						} else {
							c.Run(base, regHost).
								AssertFail()
						}

						// Explicit "no insecure" flag should always fail here since we do not have access to the CA
						(&Client{}).
							WithCredentials(username, password).
							WithInsecure(false).
							Run(base, regHost).
							AssertFail()

						// Always succeed with insecure
						(&Client{}).
							WithCredentials(username, password).
							WithInsecure(true).
							Run(base, regHost).
							AssertOK()
					})

					t.Run("Valid credentials (with access to server CA)", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir)

						rl, _ := dockerutil.Parse(regHost)
						if rl.IsLocalhost() || tls {
							c.Run(base, regHost).
								AssertOK()
						} else {
							c.Run(base, regHost).
								AssertFail()
						}

						// Succeeds only if the server uses TLS
						if tls {
							c.WithInsecure(false).
								Run(base, regHost).
								AssertOK()
						} else {
							c.WithInsecure(false).
								Run(base, regHost).
								AssertFail()
						}

						c.WithInsecure(true).
							Run(base, regHost).
							AssertOK()
					})

					t.Run("Valid credentials, any url variant, should always succeed", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir).
							// Just use insecure here for all servers - it does not matter for what we are testing here
							// in this case, which is whether we can successfully log in against any of these variants
							WithInsecure(true)

						c.Run(base, "http://"+regHost).AssertOK()
						c.Run(base, "https://"+regHost).AssertOK()
						c.Run(base, "http://"+regHost+"/whatever?foo=bar;foo:bar#foo=bar").AssertOK()
						c.Run(base, "https://"+regHost+"/whatever?foo=bar&bar=foo;foo=foo+bar:bar#foo=bar").AssertOK()
					})

					t.Run("Wrong password should always fail", func(t *testing.T) {
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

					t.Run("Wrong username should always fail", func(t *testing.T) {
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
