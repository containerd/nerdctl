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

package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
	"gotest.tools/v3/icmd"
)

func safeRandomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	// XXX WARNING there is something in the registry (or more likely in the way we generate htpasswd files)
	// that is broken and does not resist truly random strings
	// return string(b)
	return base64.URLEncoding.EncodeToString(b)
}

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
	ag.args = append(ag.args, "--username", username, "--password", password)
	return ag
}

func (ag *Client) Run(base *testutil.Base, host string) *testutil.Cmd {
	if ag.configPath == "" {
		ag.configPath, _ = os.MkdirTemp(base.T.TempDir(), "docker-config")
	}
	args := append([]string{"--debug-full", "login"}, ag.args...)
	icmdCmd := icmd.Command(base.Binary, append(base.Args, append(args, host)...)...)
	icmdCmd.Env = append(base.Env, "HOME="+os.Getenv("HOME"), "DOCKER_CONFIG="+ag.configPath)
	return &testutil.Cmd{
		Cmd:  icmdCmd,
		Base: base,
	}
}

func TestLogin(t *testing.T) {
	// Skip docker, because Docker doesn't have `--hosts-dir` nor `insecure-registry` option
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	t.Parallel()

	testregistry.EnsureImages(base)

	testCases := []struct {
		port     int
		tls      bool
		auth     string
		insecure bool
	}{
		{
			80,
			false,
			"basic",
			true,
		},
		{
			443,
			false,
			"basic",
			true,
		},
		{
			0,
			false,
			"basic",
			true,
		},
		{
			80,
			true,
			"basic",
			false,
		},
		{
			443,
			true,
			"basic",
			false,
		},
		{
			0,
			true,
			"basic",
			false,
		},
		{
			80,
			false,
			"token",
			true,
		},
		{
			443,
			false,
			"token",
			true,
		},
		{
			0,
			false,
			"token",
			true,
		},
		{
			80,
			true,
			"token",
			false,
		},
		{
			443,
			true,
			"token",
			false,
		},
		{
			0,
			true,
			"token",
			false,
		},
	}

	for _, tc := range testCases {
		// Since we have a lock mechanism for acquiring ports, we can just parallelize everything
		t.Run(fmt.Sprintf("Login against registry with tls: %t port: %d auth: %s", tc.tls, tc.port, tc.auth), func(t *testing.T) {
			// Tests with fixed ports should not be parallelized (although the port locking mechanism will prevent conflicts)
			// as their children are, and this might deadlock given how Parallel works
			if tc.port == 0 {
				t.Parallel()
			}

			// Generate credentials so that we never cross hit another test registry (spiced up with unicode)
			// Note that the grammar for basic auth does not allow colons in usernames, while token auth allows it
			username := safeRandomString(30) + "∞"
			password := safeRandomString(30) + ":∞"

			// Get a CA if we want TLS
			var ca *testca.CA
			if tc.tls {
				ca = testca.New(base.T)
			}

			// Add the requested authentication
			var auth testregistry.Auth
			auth = &testregistry.NoAuth{}
			var dependentCleanup func(error)
			if tc.auth == "basic" {
				auth = &testregistry.BasicAuth{
					Username: username,
					Password: password,
				}
			} else if tc.auth == "token" {
				authCa := ca
				// We could be on !tls - still need a ca to sign jwt
				if authCa == nil {
					authCa = testca.New(base.T)
				}
				as := testregistry.NewAuthServer(base, authCa, 0, username, password, tc.tls)
				auth = &testregistry.TokenAuth{
					Address:  as.Scheme + "://" + net.JoinHostPort(as.IP.String(), strconv.Itoa(as.Port)),
					CertPath: as.CertPath,
				}
				dependentCleanup = as.Cleanup
			}

			// Start the registry
			reg := testregistry.NewRegistry(base, ca, tc.port, auth, dependentCleanup)

			// Attach our cleanup function
			t.Cleanup(func() {
				reg.Cleanup(nil)
			})

			regHosts := []string{
				net.JoinHostPort(reg.IP.String(), strconv.Itoa(reg.Port)),
			}

			// XXX seems like omitting ports is broken on main currently
			// (plus the hosts.toml resolution is not good either)
			// XXX we should also add hostname here (maybe use the container name?)
			// Obviously also need to add localhost to the mix once we fix behavior
			/*
				if reg.Port == 443 || reg.Port == 80 {
					regHosts = append(regHosts, reg.IP.String())
				}
			*/

			for _, value := range regHosts {
				regHost := value
				t.Run(regHost, func(t *testing.T) {
					t.Parallel()

					t.Run("Valid credentials (no certs) ", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, password)

						// Fail without insecure
						c.Run(base, regHost).AssertFail()

						// Succeed with insecure
						c.WithInsecure(true).
							Run(base, regHost).AssertOK()
					})

					t.Run("Valid credentials (with certs)", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir)

						if tc.insecure {
							c.Run(base, regHost).AssertFail()
						} else {
							c.Run(base, regHost).AssertOK()
						}

						c.WithInsecure(true).
							Run(base, regHost).AssertOK()
					})

					t.Run("Valid credentials (with certs), any variant", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, password).
							WithHostsDir(reg.HostsDir).
							// Just use insecure here for all servers - it does not matter for what we are testing here
							WithInsecure(true)

						c.Run(base, "http://"+regHost).AssertOK()
						c.Run(base, "https://"+regHost).AssertOK()
						c.Run(base, "http://"+regHost+"/whatever?foo=bar;foo:bar#foo=bar").AssertOK()
						c.Run(base, "https://"+regHost+"/whatever?foo=bar&bar=foo;foo=foo+bar:bar#foo=bar").AssertOK()
					})

					t.Run("Wrong pass (no certs)", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, "invalid")

						c.Run(base, regHost).AssertFail()

						c.WithInsecure(true).
							Run(base, regHost).AssertFail()
					})

					t.Run("Wrong user (no certs)", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials("invalid", password)

						c.Run(base, regHost).AssertFail()

						c.WithInsecure(true).
							Run(base, regHost).AssertFail()
					})

					t.Run("Wrong pass (with certs)", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials(username, "invalid").
							WithHostsDir(reg.HostsDir)

						c.Run(base, regHost).AssertFail()

						c.WithInsecure(true).
							Run(base, regHost).AssertFail()
					})

					t.Run("Wrong user (with certs)", func(t *testing.T) {
						t.Parallel()
						c := (&Client{}).
							WithCredentials("invalid", password).
							WithHostsDir(reg.HostsDir)

						c.Run(base, regHost).AssertFail()

						c.WithInsecure(true).
							Run(base, regHost).AssertFail()
					})
				})
			}
		})
	}
}
