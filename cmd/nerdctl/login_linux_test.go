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
