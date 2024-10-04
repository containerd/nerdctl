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
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func Setup() {
	test.CustomCommand(nerdctlSetup)
}

// Nerdctl specific config key and values
var NerdctlToml test.ConfigKey = "NerdctlToml"
var DockerConfig test.ConfigKey = "DockerConfig"
var HostsDir test.ConfigKey = "HostsDir"
var DataRoot test.ConfigKey = "DataRoot"
var Namespace test.ConfigKey = "Namespace"

type NerdCommand struct {
	test.GenericCommand
	// FIXME: annoying - forces custom Clone, etc
	Target string
}

// Run does override the generic command run, as we are testing both docker and nerdctl
func (nc *NerdCommand) Run(expect *test.Expected) {
	// We are not in the business of testing docker error output, so, spay expect for errors testing, if any
	if expect != nil && nc.Target != testutil.Nerdctl {
		expect.Errors = nil
	}

	nc.GenericCommand.Run(expect)
}

// Clone is overridden as well, as we need to pass along the target
func (nc *NerdCommand) Clone() test.Command {
	return &NerdCommand{
		GenericCommand: *((nc.GenericCommand.Clone()).(*test.GenericCommand)),
		Target:         nc.Target,
	}
}

func nerdctlSetup(testCase *test.Case, t *testing.T) test.Command {
	t.Helper()

	var testUtilBase *testutil.Base
	dt := testCase.Data
	var pvNamespace string
	inherited := false

	if dt.ReadConfig(ipv6) != only && testutil.GetEnableIPv6() {
		t.Skip("runner skips non-IPv6 compatible tests in the IPv6 environment")
	}

	if dt.ReadConfig(mode) == modePrivate {
		// If private was inherited, we already got a configured namespace
		if dt.ReadConfig(Namespace) != "" {
			pvNamespace = string(dt.ReadConfig(Namespace))
			inherited = true
		} else {
			// Otherwise, we need to set everything up
			pvNamespace = testCase.Data.Identifier()
			dt.WithConfig(Namespace, test.ConfigValue(pvNamespace))
			testCase.Env["DOCKER_CONFIG"] = testCase.Data.TempDir()
			testCase.Env["NERDCTL_TOML"] = filepath.Join(testCase.Data.TempDir(), "nerdctl.toml")
			dt.WithConfig(HostsDir, test.ConfigValue(testCase.Data.TempDir()))
			// Setting data root is more trouble than anything and does not significantly increase isolation
			// dt.WithConfig(DataRoot, test.ConfigValue(testCase.Data.TempDir()))
		}
		testUtilBase = testutil.NewBaseWithNamespace(t, pvNamespace)
		if testUtilBase.Target == testutil.Docker {
			// For docker, just disable parallel
			testCase.NoParallel = true
		}
	} else if dt.ReadConfig(Namespace) != "" {
		pvNamespace = string(dt.ReadConfig(Namespace))
		testUtilBase = testutil.NewBaseWithNamespace(t, pvNamespace)
	} else {
		testUtilBase = testutil.NewBase(t)
	}

	// If we were passed custom content for NerdctlToml, save it
	// Not happening if this is not nerdctl of course
	if testUtilBase.Target == testutil.Nerdctl {
		if dt.ReadConfig(NerdctlToml) != "" {
			dest := filepath.Join(testCase.Data.TempDir(), "nerdctl.toml")
			testCase.Env["NERDCTL_TOML"] = dest
			err := os.WriteFile(dest, []byte(dt.ReadConfig(NerdctlToml)), 0400)
			assert.NilError(t, err, "failed to write custom nerdctl toml file for test")
		}
		if dt.ReadConfig(DockerConfig) != "" {
			dest := filepath.Join(testCase.Data.TempDir(), "config.json")
			testCase.Env["DOCKER_CONFIG"] = filepath.Dir(dest)
			err := os.WriteFile(dest, []byte(dt.ReadConfig(DockerConfig)), 0400)
			assert.NilError(t, err, "failed to write custom docker config json file for test")
		}
	}

	// Build the base
	baseCommand := &NerdCommand{}
	baseCommand.WithBinary(testUtilBase.Binary)
	baseCommand.WithArgs(testUtilBase.Args...)
	baseCommand.WithEnv(testCase.Env)
	baseCommand.WithT(t)
	baseCommand.WithTempDir(testCase.Data.TempDir())
	baseCommand.Target = testUtilBase.Target

	if testUtilBase.Target == testutil.Nerdctl {
		if dt.ReadConfig(HostsDir) != "" {
			baseCommand.WithArgs("--hosts-dir=" + string(dt.ReadConfig(HostsDir)))
		}

		if dt.ReadConfig(DataRoot) != "" {
			baseCommand.WithArgs("--data-root=" + string(dt.ReadConfig(DataRoot)))
		}
	}

	// If we were in a custom namespace, not inherited - make sure we clean up the namespace
	if testUtilBase.Target == testutil.Nerdctl && pvNamespace != "" && !inherited {
		cleanup := func() {
			// Stop all containers, then prune everything
			containerList := baseCommand.Clone()
			containerList.WithArgs("ps", "-q")
			containerList.Run(&test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					if stdout != "" {
						containerRm := baseCommand.Clone()
						containerRm.WithArgs("rm", "-f", stdout)
						containerRm.Run(&test.Expected{})
					}
				},
			})

			systemPrune := baseCommand.Clone()
			systemPrune.WithArgs("system", "prune", "-f", "--all", "--volumes")
			systemPrune.Run(&test.Expected{})

			cleanNamespace := baseCommand.Clone()
			cleanNamespace.WithArgs("namespace", "remove", pvNamespace)
			cleanNamespace.Run(nil)
		}
		cleanup()
		t.Cleanup(cleanup)
	}

	// Attach the base command
	return baseCommand
}
