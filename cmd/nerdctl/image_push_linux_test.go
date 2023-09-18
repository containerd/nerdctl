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
	"net/http"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
	"gotest.tools/v3/assert"
)

func TestPushPlainHTTPFails(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base, 5000)
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
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base, 5000)
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
	testutil.RequiresBuild(t)
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base, 5000)
	defer reg.Cleanup()

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
}

func TestPushPlainHttpInsecureWithDefaultPort(t *testing.T) {
	testutil.RequiresBuild(t)
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base, 80)
	defer reg.Cleanup()

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s/%s:%s",
		reg.IP.String(), testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
}

func TestPushInsecureWithLogin(t *testing.T) {
	testutil.RequiresBuild(t)
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
	testutil.RequiresBuild(t)
	// Skip docker, because Docker doesn't have `--hosts-dir` option, and we don't want to contaminate the global /etc/docker/certs.d during this test
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewHTTPS(base, "admin", "badmin")
	defer reg.Cleanup()

	base.Cmd("--hosts-dir", reg.HostsDir, "login", "-u", "admin", "-p", "badmin", fmt.Sprintf("%s:%d", reg.IP.String(), reg.ListenPort)).AssertOK()

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.CommonImage, testImageRef).AssertOK()

	base.Cmd("--debug", "--hosts-dir", reg.HostsDir, "push", testImageRef).AssertOK()
}

func TestPushNonDistributableArtifacts(t *testing.T) {
	testutil.RequiresBuild(t)
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	// Skip docker, because "--allow-nondistributable-artifacts" is a daemon-only option and requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := testregistry.NewPlainHTTP(base, 5000)
	defer reg.Cleanup()

	base.Cmd("pull", testutil.NonDistBlobImage).AssertOK()

	testImgRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.NonDistBlobImage, ":")[1])
	base.Cmd("tag", testutil.NonDistBlobImage, testImgRef).AssertOK()

	base.Cmd("--debug", "--insecure-registry", "push", testImgRef).AssertOK()

	blobURL := fmt.Sprintf("http://%s:%d/v2/%s/blobs/%s", reg.IP.String(), reg.ListenPort, testutil.Identifier(t), testutil.NonDistBlobDigest)
	resp, err := http.Get(blobURL)
	assert.Assert(t, err, "error making http request")
	if resp.Body != nil {
		resp.Body.Close()
	}
	assert.Equal(t, resp.StatusCode, http.StatusNotFound, "non-distributable blob should not be available")

	base.Cmd("--debug", "--insecure-registry", "push", "--allow-nondistributable-artifacts", testImgRef).AssertOK()
	resp, err = http.Get(blobURL)
	assert.Assert(t, err, "error making http request")
	if resp.Body != nil {
		resp.Body.Close()
	}
	assert.Equal(t, resp.StatusCode, http.StatusOK, "non-distributable blob should be available")
}

func TestPushSoci(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	requiresSoci(base)
	reg := testregistry.NewPlainHTTP(base, 5000)
	defer reg.Cleanup()

	base.Cmd("pull", testutil.UbuntuImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/%s:%s",
		reg.IP.String(), reg.ListenPort, testutil.Identifier(t), strings.Split(testutil.UbuntuImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.UbuntuImage, testImageRef).AssertOK()

	base.Cmd("--snapshotter=soci", "--insecure-registry", "push", "--soci-span-size=2097152", "--soci-min-layer-size=20971520", testImageRef).AssertOK()
}
