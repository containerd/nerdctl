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
	"errors"
	"os"
	"path/filepath"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

type nerdCommand struct {
	test.GenericCommand

	DockerConfig string
	Namespace    string
	NerdctlToml  string
	HostsDir     string
	DataRoot     string
	Debug        bool
}

// Run does override the generic command run, as we are testing both docker and nerdctl
func (nc *nerdCommand) Run(expect *test.Expected) {
	if nc.T() != nil {
		nc.T().Helper()
	}

	// If no DOCKER_CONFIG was explicitly provided, set ourselves inside the current working directory
	// Note that subtests do then inherit parent test config by default, unless overridden
	if nc.Env["DOCKER_CONFIG"] == "" {
		nc.Env["DOCKER_CONFIG"] = nc.TempDir()
	}

	if nc.DockerConfig != "" {
		dest := filepath.Join(nc.Env["DOCKER_CONFIG"], "config.json")
		if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
			err := os.WriteFile(dest, []byte(nc.DockerConfig), 0400)
			if nc.GenericCommand.T() != nil {
				assert.NilError(nc.T(), err, "failed to write custom docker config json file for test")
			}
		}
	}

	// We are not in the business of testing docker *error* output, so, spay expectation here (for errors)
	if GetTarget() != targetNerdctl {
		if expect != nil {
			expect.Errors = nil
		}

		if nc.Debug {
			nc.PrependArgs("--log-level=debug")
		}
	} else {
		// Set the namespace
		if nc.Namespace != "" {
			nc.PrependArgs("--namespace=" + nc.Namespace)
		}

		// If no NERDCTL_TOML was explicitly provided, set it to the private dir
		if nc.Env["NERDCTL_TOML"] == "" {
			nc.Env["NERDCTL_TOML"] = filepath.Join(nc.TempDir(), "nerdctl.toml")
		}

		// If we have custom toml content, write it if it does not exist already
		if nc.NerdctlToml != "" {
			dest := nc.Env["NERDCTL_TOML"]
			if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
				err := os.WriteFile(dest, []byte(nc.NerdctlToml), 0400)
				if nc.GenericCommand.T() != nil {
					assert.NilError(nc.GenericCommand.T(), err, "failed to write NerdctlToml")
				}
			}
		}

		if nc.HostsDir != "" {
			nc.PrependArgs("--hosts-dir=" + nc.HostsDir)
		}

		if nc.DataRoot != "" {
			nc.PrependArgs("--data-root=" + nc.DataRoot)
		}

		if nc.Debug {
			nc.PrependArgs("--debug-full")
		}
	}

	nc.GenericCommand.Run(expect)
}

func (nc *nerdCommand) Clone() test.Command {
	return &nerdCommand{
		GenericCommand: *((nc.GenericCommand.Clone()).(*test.GenericCommand)),
		Namespace:      nc.Namespace,
		NerdctlToml:    nc.NerdctlToml,
		HostsDir:       nc.HostsDir,
		DataRoot:       nc.DataRoot,
		Debug:          nc.Debug,
	}
}
