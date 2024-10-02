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
	"strconv"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/ca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func NewKuboRegistry(data test.Data, helpers test.Helpers, t *testing.T, currentCA *ca.CA, port int, auth Auth) *Server {
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(t, err, fmt.Errorf("failed finding ipv4 non loopback interface: %w", err))
	port, err = portlock.Acquire(port)
	assert.NilError(t, err, fmt.Errorf("failed acquiring port: %w", err))

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
		testutil.KuboImage,
		"-c", "--",
		fmt.Sprintf("ipfs init && ipfs config Addresses.API /ip4/0.0.0.0/tcp/%d && ipfs daemon --offline", port),
	}

	scheme := "http"

	cleanup := func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", containerName)
		errPortRelease := portlock.Release(port)

		assert.NilError(t, errPortRelease, fmt.Errorf("failed releasing port: %w", err))
	}

	setup := func(data test.Data, helpers test.Helpers) {
		helpers.Ensure(args...)
		ensureContainerStarted(helpers, containerName)
		_, err = nettestutil.HTTPGet(fmt.Sprintf("%s://%s/api/v0",
			scheme,
			net.JoinHostPort(hostIP.String(), strconv.Itoa(port)),
		),
			10,
			true)
		assert.NilError(t, err, fmt.Errorf("failed starting kubo registry in a timely manner: %w", err))
	}

	return &Server{
		IP:      hostIP,
		Port:    port,
		Scheme:  scheme,
		Cleanup: cleanup,
		Setup:   setup,
		Logs: func(data test.Data, helpers test.Helpers) {
			// FIXME: get rid of unbuffer and allow manipulating stderr
			cmd := helpers.Command("logs", containerName)
			cmd.WithWrapper("unbuffer")
			cmd.Run(&test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					t.Logf("%s: %q", containerName, stdout)
				},
			})
		},
	}
}
