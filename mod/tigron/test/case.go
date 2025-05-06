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

package test

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/internal/formatter"
)

// Case describes an entire test-case, including data, setup and cleanup routines, command and
// expectations.
type Case struct {
	// Description contains a human-readable short desc, used as a seed for the identifier and as a
	// title for the test.
	Description string
	// NoParallel disables parallel execution if set to true.
	// This obviously implies that all tests run in parallel, by default. This is a design choice.
	NoParallel bool
	// Env contains a map of environment variables to use as a base for all commands run in Setup,
	// Command and Cleanup.
	// Note that the environment is inherited by subtests.
	Env map[string]string
	// Data contains test specific data, accessible to all operations, also inherited by subtests.
	Data Data
	// Config contains specific information meaningful to the binary being tested.
	// It is also inherited by subtests.
	Config Config

	// Requirement
	Require *Requirement
	// Setup
	Setup Butler
	// Command
	Command Executor
	// Expected
	Expected Manager
	// Cleanup
	Cleanup Butler

	// SubTests
	SubTests []*Case

	// Private
	helpers Helpers
	t       *testing.T
	parent  *Case
}

const (
	startDecorator  = "🚀"
	cleanDecorator  = "🧽"
	setupDecorator  = "🏗"
	subinDecorator  = "⤵️"
	suboutDecorator = "↩️"
	tempDecorator   = "⏳"
)

// Run prepares and executes the test, and any possible subtests.
func (test *Case) Run(t *testing.T) {
	t.Helper()
	// Run the test
	//nolint:thelper
	testRun := func(subT *testing.T) {
		subT.Helper()

		silentT := assertive.WithSilentSuccess(subT)

		assertive.True(silentT, test.t == nil, "You cannot run a test multiple times")
		assertive.True(silentT, test.Description != "" || test.parent == nil,
			"A subtest description cannot be empty")
		assertive.True(silentT, test.Command == nil || test.Expected != nil,
			"Expectations for a test command cannot be nil. You may want to use `Setup` instead"+
				"of `Command`.")

		// Attach testing.T
		test.t = subT

		// Ensure we have env
		if test.Env == nil {
			test.Env = map[string]string{}
		}

		// If we have a parent, get parent env, data and config
		var parentData Data

		var parentConfig Config

		if test.parent != nil {
			parentData = test.parent.Data
			parentConfig = test.parent.Config

			for k, v := range test.parent.Env {
				if _, ok := test.Env[k]; !ok {
					test.Env[k] = v
				}
			}
		}

		// Inherit and attach Data and Config
		test.Data = newData(test.t, test.Data, parentData)
		test.Config = configureConfig(test.Config, parentConfig)

		var custCom CustomizableCommand
		if registeredTestable == nil {
			custCom = NewGenericCommand()
		} else {
			custCom = registeredTestable.CustomCommand(test, test.t)
		}

		// Separate cwd from the temp directory
		custCom.WithCwd(test.t.TempDir())
		custCom.withT(test.t)
		// Set the command tempdir to another temp location.
		// This is required for the current extension mechanism to allow creation of command dependent configuration
		// assets. Note that this is a different location than both CWD and Data.Temp().Path().
		custCom.withTempDir(test.t.TempDir())
		custCom.withEnv(test.Env)
		custCom.withConfig(test.Config)

		// Attach the base command, and t
		test.helpers = &helpersInternal{
			cmdInternal: custCom,
			t:           test.t,
		}

		setups := []func(data Data, helpers Helpers){}
		cleanups := []func(data Data, helpers Helpers){}

		// Check the requirements before going any further
		if test.Require != nil {
			shouldRun, message := test.Require.Check(test.Data, test.helpers)
			if !shouldRun {
				test.t.Skipf("test skipped as: %s", message)
			}

			if test.Require.Setup != nil {
				setups = append(setups, test.Require.Setup)
			}

			if test.Require.Cleanup != nil {
				cleanups = append(cleanups, test.Require.Cleanup)
			}
		}

		// Register setup if any
		if test.Setup != nil {
			setups = append(setups, test.Setup)
		}

		// Register cleanup if any
		if test.Cleanup != nil {
			cleanups = append(cleanups, test.Cleanup)
		}

		// Run optional post requirement hook
		if registeredTestable != nil {
			registeredTestable.AmbientRequirements(test, test.t)
		}

		// Set parallel unless asked not to
		if !test.NoParallel {
			test.t.Parallel()
		}

		// Execute cleanups now
		if len(cleanups) > 0 {
			test.t.Log(
				"\n\n" + formatter.Table(
					[][]any{{cleanDecorator, fmt.Sprintf("%q: initial cleanup", test.t.Name())}},
					"=",
				) + "\n",
			)

			for _, cleanup := range cleanups {
				cleanup(test.Data, test.helpers)
			}

			// Register the cleanups, in reverse
			test.t.Cleanup(func() {
				test.t.Helper()
				test.t.Log(
					"\n\n" + formatter.Table(
						[][]any{{cleanDecorator, fmt.Sprintf("%q: post-cleanup", test.t.Name())}},
						"=",
					) + "\n",
				)

				slices.Reverse(cleanups)

				for _, cleanup := range cleanups {
					cleanup(test.Data, test.helpers)
				}
			})
		}

		// Run the setups
		if len(setups) > 0 {
			test.t.Log(
				"\n\n" + formatter.Table(
					[][]any{{setupDecorator, fmt.Sprintf("%q: setup", test.t.Name())}},
					"=",
				) + "\n",
			)

			for _, setup := range setups {
				setup(test.Data, test.helpers)
			}
		}

		// Run the command if any, with expectations
		// Note: if we have a command, we already know we DO have Expected
		if test.Command != nil {
			cmd := test.Command(test.Data, test.helpers)

			debugConfig, _ := json.MarshalIndent(test.Config.(*config).config, "", "  ")
			debugData, _ := json.MarshalIndent(test.Data.(*data).labels, "", "  ")

			// Show the files in the temp directory BEFORE the command is executed
			tempFiles := []string{}

			if files, err := os.ReadDir(test.Data.Temp().Path()); err == nil {
				for _, file := range files {
					tempFiles = append(tempFiles, file.Name())
				}
			}

			test.t.Log(
				"\n\n" + formatter.Table(
					[][]any{
						{startDecorator, fmt.Sprintf("%q: starting test!", test.t.Name())},
						{tempDecorator, test.Data.Temp().Dir()},
						{"", strings.Join(tempFiles, "\n")},
						{"config", string(debugConfig)},
						{"labels", string(debugData)},
					},
					"=",
				) + "\n",
			)
			// FIXME: so, the expected function will run BEFORE the command
			cmd.Run(test.Expected(test.Data, test.helpers))
		}

		if len(test.SubTests) > 0 {
			// Now go for the subtests
			test.t.Logf("\n%s️ %q: into subtests prep", subinDecorator, test.t.Name())

			for _, subTest := range test.SubTests {
				subTest.parent = test
				subTest.Run(test.t)
			}

			test.t.Logf("\n%s️ %q: done with subtests prep", suboutDecorator, test.t.Name())
		}
	}

	if test.parent != nil {
		t.Run(test.Description, testRun)
	} else {
		testRun(t)
	}
}
