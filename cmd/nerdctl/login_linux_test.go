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
	"fmt"
	"net"
	"path"
	"strconv"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
)

func TestLogin(t *testing.T) {
	// Skip docker, because Docker doesn't have `--hosts-dir` option, and we don't want to contaminate the global /etc/docker/certs.d during this test
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewHTTPS(base, "admin", "validTestPassword")
	defer reg.Cleanup()

	regHost := net.JoinHostPort(reg.IP.String(), strconv.Itoa(reg.ListenPort))

	t.Logf("Good password")
	base.Cmd("--debug-full", "--hosts-dir", reg.HostsDir, "login", "-u", "admin", "-p", "validTestPassword", regHost).AssertOK()

	t.Logf("Bad password")
	base.Cmd("--debug-full", "--hosts-dir", reg.HostsDir, "login", "-u", "admin", "-p", "invalidTestPassword", regHost).AssertFail()
}

func TestLoginWithSpecificRegHosts(t *testing.T) {
	// Skip docker, because Docker doesn't have `--hosts-dir` option, and we don't want to contaminate the global /etc/docker/certs.d during this test
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewHTTPS(base, "admin", "validTestPassword")
	defer reg.Cleanup()

	regHost := net.JoinHostPort(reg.IP.String(), strconv.Itoa(reg.ListenPort))

	t.Logf("Prepare regHost URL with path and Scheme")

	type testCase struct {
		url string
		log string
	}
	testCases := []testCase{
		{
			url: "https://" + path.Join(regHost, "test"),
			log: "Login with repository containing path and scheme in the URL",
		},
		{
			url: path.Join(regHost, "test"),
			log: "Login with repository containing path and without scheme in the URL",
		},
	}
	for _, tc := range testCases {
		t.Logf(tc.log)
		base.Cmd("--debug-full", "--hosts-dir", reg.HostsDir, "login", "-u", "admin", "-p", "validTestPassword", tc.url).AssertOK()
	}

}

func TestLoginWithPlainHttp(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	reg5000 := testregistry.NewAuthWithHTTP(base, "admin", "validTestPassword", 5000, 5001)
	reg80 := testregistry.NewAuthWithHTTP(base, "admin", "validTestPassword", 80, 5002)
	defer reg5000.Cleanup()
	defer reg80.Cleanup()
	testCasesForPort5000 := []struct {
		regHost           string
		regPort           int
		useRegPort        bool
		username          string
		password          string
		shouldSuccess     bool
		registry          *testregistry.TestRegistry
		shouldUseInSecure bool
	}{
		{
			regHost:           "127.0.0.1",
			regPort:           5000,
			useRegPort:        true,
			username:          "admin",
			password:          "validTestPassword",
			shouldSuccess:     true,
			registry:          reg5000,
			shouldUseInSecure: true,
		},
		{
			regHost:           "127.0.0.1",
			regPort:           5000,
			useRegPort:        true,
			username:          "admin",
			password:          "invalidTestPassword",
			shouldSuccess:     false,
			registry:          reg5000,
			shouldUseInSecure: true,
		},
		{
			regHost:    "127.0.0.1",
			regPort:    5000,
			useRegPort: true,
			username:   "admin",
			password:   "validTestPassword",
			// Following the merging of the below, any localhost/loopback registries will
			// get automatically downgraded to HTTP so this will still succceed:
			// https://github.com/containerd/containerd/pull/7393
			shouldSuccess:     true,
			registry:          reg5000,
			shouldUseInSecure: false,
		},
		{
			regHost:           "127.0.0.1",
			regPort:           80,
			useRegPort:        false,
			username:          "admin",
			password:          "validTestPassword",
			shouldSuccess:     true,
			registry:          reg80,
			shouldUseInSecure: true,
		},
		{
			regHost:           "127.0.0.1",
			regPort:           80,
			useRegPort:        false,
			username:          "admin",
			password:          "invalidTestPassword",
			shouldSuccess:     false,
			registry:          reg80,
			shouldUseInSecure: true,
		},
		{
			regHost:    "127.0.0.1",
			regPort:    80,
			useRegPort: false,
			username:   "admin",
			password:   "validTestPassword",
			// Following the merging of the below, any localhost/loopback registries will
			// get automatically downgraded to HTTP so this will still succceed:
			// https://github.com/containerd/containerd/pull/7393
			shouldSuccess:     true,
			registry:          reg80,
			shouldUseInSecure: false,
		},
	}
	for _, tc := range testCasesForPort5000 {
		tcName := fmt.Sprintf("%+v", tc)
		t.Run(tcName, func(t *testing.T) {
			regHost := tc.regHost
			if tc.useRegPort {
				regHost = fmt.Sprintf("%s:%d", regHost, tc.regPort)
			}
			if tc.shouldSuccess {
				t.Logf("Good password")
			} else {
				t.Logf("Bad password")
			}
			var args []string
			if tc.shouldUseInSecure {
				args = append(args, "--insecure-registry")
			}
			args = append(args, []string{
				"--debug-full", "--hosts-dir", tc.registry.HostsDir, "login", "-u", tc.username, "-p", tc.password, regHost,
			}...)
			cmd := base.Cmd(args...)
			if tc.shouldSuccess {
				cmd.AssertOK()
			} else {
				cmd.AssertFail()
			}
		})
	}
}
