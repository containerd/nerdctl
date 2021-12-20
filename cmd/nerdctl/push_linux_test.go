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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestPushPlainHTTPFails(t *testing.T) {
	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base)
	defer reg.Cleanup()

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	res := base.Cmd("push", testImageRef).Run()
	resCombined := res.Combined()
	t.Logf("result: exitCode=%d, out=%q", res.ExitCode, res.Combined())
	assert.Assert(t, res.ExitCode != 0)
	assert.Assert(t, strings.Contains(resCombined, "server gave HTTP response to HTTPS client"))
}

func TestPushPlainHTTPLocalhost(t *testing.T) {
	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base)
	defer reg.Cleanup()
	localhostIP := "127.0.0.1"
	t.Logf("localhost IP=%q", localhostIP)

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		localhostIP, reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	base.Cmd("push", testImageRef).AssertOK()
}

func TestPushPlainHTTPInsecure(t *testing.T) {
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base)
	defer reg.Cleanup()

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
}

func TestPushInsecureWithLogin(t *testing.T) {
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewHTTPS(base, "admin", "badmin")
	defer reg.Cleanup()

	base.Cmd("--insecure-registry", "login", "-u", "admin", "-p", "badmin",
		fmt.Sprintf("%s:%d", reg.IP.String(), reg.ListenPort)).AssertOK()
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	base.Cmd("push", testImageRef).AssertFail()
	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
}

func TestPushWithHostsDir(t *testing.T) {
	// Skip docker, because Docker doesn't have `--hosts-dir` option, and we don't want to contaminate the global /etc/docker/certs.d during this test
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewHTTPS(base, "admin", "badmin")
	defer reg.Cleanup()

	// FIXME: `nerdctl login` still ignores hosts-dir (msg="Get \"https://192.168.12.34:5000/v2/\": x509: certificate signed by unknown authority")
	// base.Cmd("--hosts-dir", reg.HostsDir, "login", "-u", "admin", "-p", "badmin", fmt.Sprintf("%s:%d", reg.IP.String(), reg.ListenPort)).AssertOK()

	res := base.Cmd("--insecure-registry", "login", "-u", "admin", "-p", "badmin", fmt.Sprintf("%s:%d", reg.IP.String(), reg.ListenPort)).Run()
	reg.Logs()
	res.Assert(t, icmd.Expected{ExitCode: 0})

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	base.Cmd("--debug", "--hosts-dir", reg.HostsDir, "push", testImageRef).AssertOK()
}
