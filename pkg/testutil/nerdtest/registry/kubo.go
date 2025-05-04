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

package registry

import (
	"fmt"
	"net"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/utils/testca"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/platform"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

func NewKuboRegistry(data test.Data, helpers test.Helpers, currentCA *testca.Cert, port int, auth Auth) *Server {
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(helpers.T(), err, fmt.Errorf("failed finding ipv4 non loopback interface: %w", err))
	port, err = portlock.Acquire(port)
	assert.NilError(helpers.T(), err, fmt.Errorf("failed acquiring port: %w", err))

	containerName := data.Identifier(fmt.Sprintf("kubo-registry-server-%d-%t", port, currentCA != nil))
	// Cleanup possible leftovers first
	helpers.Ensure("rm", "-f", containerName)

	args := []string{
		"run",
		"--pull=never",
		"-d",
		"-p", fmt.Sprintf("%s:%d:%d", listenIP, port, port),
		"--name", containerName,
		"--entrypoint=/bin/sh",
		platform.KuboImage,
		"-c", "--",
		fmt.Sprintf("ipfs init && ipfs config Addresses.API /ip4/0.0.0.0/tcp/%d && ipfs daemon --offline", port),
	}

	scheme := "http"

	cleanup := func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", containerName)
		errPortRelease := portlock.Release(port)

		assert.NilError(helpers.T(), errPortRelease, fmt.Errorf("failed releasing port: %w", err))
	}

	setup := func(data test.Data, helpers test.Helpers) {
		helpers.Ensure(args...)
		ensureServerStarted(helpers, containerName, scheme, hostIP, port)
	}

	return &Server{
		Scheme:  scheme,
		IP:      hostIP,
		Port:    port,
		Setup:   setup,
		Cleanup: cleanup,
	}
}
