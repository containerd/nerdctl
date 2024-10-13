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
	"os"
	"strconv"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/ca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/hoststoml"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/platform"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func NewDockerRegistry(data test.Data, helpers test.Helpers, currentCA *ca.CA, port int, auth Auth) *Server {
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(helpers.T(), err, fmt.Errorf("failed finding ipv4 non loopback interface: %w", err))
	// XXX RELEASE PORT IN CLEANUP HERE
	// FIXME: this will fail in many circumstances. Review strategy on how to acquire a free port.
	// We probably have better code for that already somewhere.
	port, err = portlock.Acquire(port)
	assert.NilError(helpers.T(), err, fmt.Errorf("failed acquiring port: %w", err))

	containerName := data.Identifier(fmt.Sprintf("docker-registry-server-%d-%t", port, currentCA != nil))
	// Cleanup possible leftovers first
	helpers.Ensure("rm", "-f", containerName)

	args := []string{
		"run",
		"--pull=never",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5000", listenIP, port),
		"--name", containerName,
	}
	scheme := "http"
	var cert *ca.Cert
	if currentCA != nil {
		scheme = "https"
		cert = currentCA.NewCert(hostIP.String(), "127.0.0.1", "localhost", "::1")
		args = append(args,
			"--env", "REGISTRY_HTTP_TLS_CERTIFICATE=/registry/domain.crt",
			"--env", "REGISTRY_HTTP_TLS_KEY=/registry/domain.key",
			"-v", cert.CertPath+":/registry/domain.crt",
			"-v", cert.KeyPath+":/registry/domain.key",
		)
	}

	// Attach authentication params returns by authenticator
	args = append(args, auth.Params(data)...)

	// Get the right registry version
	registryImage := platform.RegistryImageStable
	up := os.Getenv("DISTRIBUTION_VERSION")
	if up != "" {
		if up[0:1] != "v" {
			up = "v" + up
		}
		registryImage = platform.RegistryImageNext + up
	}
	args = append(args, registryImage)

	cleanup := func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", containerName)
		errPortRelease := portlock.Release(port)

		if cert != nil {
			assert.NilError(helpers.T(), cert.Close(), fmt.Errorf("failed cleaning certificates: %w", err))
		}

		assert.NilError(helpers.T(), errPortRelease, fmt.Errorf("failed releasing port: %w", err))
	}

	// FIXME: in the future, we will want to further manipulate hosts toml file from the test
	// This should then return the struct, instead of saving it on its own
	hostsDir, err := func() (string, error) {
		hDir, err := os.MkdirTemp(data.TempDir(), "certs.d")
		assert.NilError(helpers.T(), err, fmt.Errorf("failed creating directory certs.d: %w", err))

		if currentCA != nil {
			hostTomlContent := &hoststoml.HostsToml{
				CA: currentCA.CertPath,
			}

			err = hostTomlContent.Save(hDir, hostIP.String(), port)
			assert.NilError(helpers.T(), err, fmt.Errorf("failed creating hosts.toml file: %w", err))

			err = hostTomlContent.Save(hDir, "127.0.0.1", port)
			assert.NilError(helpers.T(), err, fmt.Errorf("failed creating hosts.toml file: %w", err))

			err = hostTomlContent.Save(hDir, "localhost", port)
			assert.NilError(helpers.T(), err, fmt.Errorf("failed creating hosts.toml file: %w", err))

			if port == 443 {
				err = hostTomlContent.Save(hDir, hostIP.String(), 0)
				assert.NilError(helpers.T(), err, fmt.Errorf("failed creating hosts.toml file: %w", err))

				err = hostTomlContent.Save(hDir, "127.0.0.1", 0)
				assert.NilError(helpers.T(), err, fmt.Errorf("failed creating hosts.toml file: %w", err))

				err = hostTomlContent.Save(hDir, "localhost", 0)
				assert.NilError(helpers.T(), err, fmt.Errorf("failed creating hosts.toml file: %w", err))

			}
		}

		return hDir, nil
	}()

	setup := func(data test.Data, helpers test.Helpers) {
		helpers.Ensure(args...)
		ensureContainerStarted(helpers, containerName)
		_, err = nettestutil.HTTPGet(fmt.Sprintf("%s://%s/v2/",
			scheme,
			net.JoinHostPort(hostIP.String(), strconv.Itoa(port)),
		),
			10,
			true)
		assert.NilError(helpers.T(), err, fmt.Errorf("failed starting docker registry in a timely manner: %w", err))
	}

	return &Server{
		Scheme:  scheme,
		IP:      hostIP,
		Port:    port,
		Cleanup: cleanup,
		Setup:   setup,
		Logs: func(data test.Data, helpers test.Helpers) {
			helpers.T().Error(helpers.Err("logs", containerName))
		},
		HostsDir: hostsDir,
	}
}
