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

package nerdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/platform"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

const defaultNamespace = testutil.Namespace

// IMPORTANT note on file writing here:
// Inside the context of a single test, there is no concurrency, as setup, command and cleanup operate in sequence
// Furthermore, the tempdir is private by definition.
// Writing files here in a non-safe manner is thus OK.
type target = string

const (
	targetNerdctl = target("nerdctl")
	targetDocker  = target("docker")
)

func getTarget() string {
	// Indirecting to testutil for now
	return testutil.GetTarget()
}

func newNerdCommand(conf test.Config, t *testing.T) *nerdCommand {
	// Decide what binary we are running
	var err error
	var binary string
	trgt := getTarget()
	switch trgt {
	case targetNerdctl:
		binary, err = exec.LookPath(trgt)
		if err != nil {
			t.Fatalf("unable to find binary %q: %v", trgt, err)
		}
		// Set the default namespace if we do not have something already
		if conf.Read(Namespace) == "" {
			conf.Write(Namespace, defaultNamespace)
		}
	case targetDocker:
		binary, err = exec.LookPath(trgt)
		if err != nil {
			t.Fatalf("unable to find binary %q: %v", trgt, err)
		}
		if err = exec.Command("docker", "compose", "version").Run(); err != nil {
			t.Fatalf("docker does not support compose: %v", err)
		}
	default:
		t.Fatalf("unknown target %q", getTarget())
	}

	// Create the base command, with the right binary, t
	ret := &nerdCommand{}
	ret.WithBinary(binary)
	// Not interested in these - and insulate us from parent environment side effects
	ret.WithBlacklist([]string{
		"LS_COLORS",
		"DOCKER_CONFIG",
		"CONTAINERD_SNAPSHOTTER",
		"NERDCTL_TOML",
		"CONTAINERD_ADDRESS",
		"CNI_PATH",
		"NETCONFPATH",
		"NERDCTL_EXPERIMENTAL",
		"NERDCTL_HOST_GATEWAY_IP",
	})
	return ret
}

type nerdCommand struct {
	test.GenericCommand

	hasWrittenToml         bool
	hasWrittenDockerConfig bool
}

func (nc *nerdCommand) Run(expect *test.Expected) {
	nc.prep()
	if getTarget() == targetDocker {
		// We are not in the business of testing docker *error* output, so, spay expectation here
		if expect != nil {
			expect.Errors = nil
		}
	}
	nc.GenericCommand.Run(expect)
}

func (nc *nerdCommand) Background(timeout time.Duration) {
	nc.prep()
	nc.GenericCommand.Background(timeout)
}

// Run does override the generic command run, as we are testing both docker and nerdctl
func (nc *nerdCommand) prep() {
	nc.T().Helper()

	// If no DOCKER_CONFIG was explicitly provided, set ourselves inside the current working directory
	if nc.Env["DOCKER_CONFIG"] == "" {
		nc.Env["DOCKER_CONFIG"] = nc.GenericCommand.TempDir
	}

	if customDCConfig := nc.GenericCommand.Config.Read(DockerConfig); customDCConfig != "" {
		if !nc.hasWrittenDockerConfig {
			dest := filepath.Join(nc.Env["DOCKER_CONFIG"], "config.json")
			err := os.WriteFile(dest, []byte(customDCConfig), 0400)
			assert.NilError(nc.T(), err, "failed to write custom docker config json file for test")
			nc.hasWrittenDockerConfig = true
		}
	}

	if getTarget() == targetDocker {
		// Allow debugging with docker syntax
		if nc.Config.Read(Debug) != "" {
			nc.PrependArgs("--log-level=debug")
		}
	} else if getTarget() == targetNerdctl {
		// Set the namespace
		if nc.Config.Read(Namespace) != "" {
			nc.PrependArgs("--namespace=" + string(nc.Config.Read(Namespace)))
		}

		if nc.Config.Read(stargz) == enabled {
			nc.Env["CONTAINERD_SNAPSHOTTER"] = "stargz"
		}

		if nc.Config.Read(ipfs) == enabled {
			var ipfsPath string
			if rootlessutil.IsRootless() {
				var err error
				ipfsPath, err = platform.DataHome()
				ipfsPath = filepath.Join(ipfsPath, "ipfs")
				assert.NilError(nc.T(), err)
			} else {
				ipfsPath = filepath.Join(os.Getenv("HOME"), ".ipfs")
			}

			nc.Env["IPFS_PATH"] = ipfsPath
		}

		// If no NERDCTL_TOML was explicitly provided, set it to the private dir
		if nc.Env["NERDCTL_TOML"] == "" {
			nc.Env["NERDCTL_TOML"] = filepath.Join(nc.GenericCommand.TempDir, "nerdctl.toml")
		}

		// If we have custom toml content, write it if it does not exist already
		if nc.Config.Read(NerdctlToml) != "" {
			if !nc.hasWrittenToml {
				dest := nc.Env["NERDCTL_TOML"]
				err := os.WriteFile(dest, []byte(nc.Config.Read(NerdctlToml)), 0400)
				assert.NilError(nc.T(), err, "failed to write NerdctlToml")
				nc.hasWrittenToml = true
			}
		}

		if nc.Config.Read(HostsDir) != "" {
			nc.PrependArgs("--hosts-dir=" + string(nc.Config.Read(HostsDir)))
		}
		if nc.Config.Read(DataRoot) != "" {
			nc.PrependArgs("--data-root=" + string(nc.Config.Read(DataRoot)))
		}
		if nc.Config.Read(Debug) != "" {
			nc.PrependArgs("--debug-full")
		}
	}
}

func (nc *nerdCommand) Clone() test.TestableCommand {
	return &nerdCommand{
		GenericCommand:         *(nc.GenericCommand.Clone().(*test.GenericCommand)),
		hasWrittenToml:         nc.hasWrittenToml,
		hasWrittenDockerConfig: nc.hasWrittenDockerConfig,
	}
}
