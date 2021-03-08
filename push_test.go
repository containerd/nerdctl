/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"strings"
	"testing"

	"github.com/AkihiroSuda/nerdctl/pkg/testutil"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
)

func getNonLoopbackIPv4() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}
		if ipv4.IsLoopback() {
			continue
		}
		return ipv4, nil
	}
	return nil, errors.Wrapf(errdefs.ErrNotFound, "non-loopback IPv4 address not found, attempted=%+v", addrs)
}

type testRegistry struct {
	ip         net.IP
	listenIP   net.IP
	listenPort int
	cleanup    func()
}

func newTestRegistry(base *testutil.Base, name string) *testRegistry {
	hostIP, err := getNonLoopbackIPv4()
	assert.NilError(base.T, err)
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	const listenPort = 5000 // TODO: choose random empty port
	base.T.Logf("hostIP=%q, listenIP=%q, listenPort=%d", hostIP, listenIP, listenPort)

	registryContainerName := "reg-" + name
	cmd := base.Cmd("run",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5000", listenIP, listenPort),
		"--name", registryContainerName,
		testutil.RegistryImage)
	cmd.AssertOK()
	if _, err = httpGet(fmt.Sprintf("http://%s:%d/v2", hostIP.String(), listenPort), 30); err != nil {
		base.Cmd("rm", "-f", registryContainerName).Run()
		base.T.Fatal(err)
	}
	return &testRegistry{
		ip:         hostIP,
		listenIP:   listenIP,
		listenPort: listenPort,
		cleanup:    func() { base.Cmd("rm", "-f", registryContainerName).Run() },
	}
}

func TestPushPlainHTTPFails(t *testing.T) {
	base := testutil.NewBase(t)
	reg := newTestRegistry(base, "test-push-plain-http-fails")
	defer reg.cleanup()

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/test-push-plain-http-fails:%s",
		reg.ip.String(), reg.listenPort, strings.Split(testutil.AlpineImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.AlpineImage, testImageRef).AssertOK()

	res := base.Cmd("push", testImageRef).Run()
	resCombined := res.Combined()
	t.Logf("result: exitCode=%d, out=%q", res.ExitCode, res.Combined())
	assert.Assert(t, res.ExitCode != 0)
	assert.Assert(t, strings.Contains(resCombined, "server gave HTTP response to HTTPS client"))
}

func TestPushPlainHTTPLocalhost(t *testing.T) {
	base := testutil.NewBase(t)
	reg := newTestRegistry(base, "test-push-plain-localhost")
	defer reg.cleanup()
	localhostIP := "127.0.0.1"
	t.Logf("localhost IP=%q", localhostIP)

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/test-push-plain-http-insecure:%s",
		localhostIP, reg.listenPort, strings.Split(testutil.AlpineImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.AlpineImage, testImageRef).AssertOK()

	base.Cmd("push", testImageRef).AssertOK()
}

func TestPushPlainHTTPInsecure(t *testing.T) {
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := newTestRegistry(base, "test-push-plain-http-insecure")
	defer reg.cleanup()

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/test-push-plain-http-insecure:%s",
		reg.ip.String(), reg.listenPort, strings.Split(testutil.AlpineImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.AlpineImage, testImageRef).AssertOK()

	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
}
