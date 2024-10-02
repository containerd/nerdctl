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
	"testing"

	"gotest.tools/v3/assert"
)

// Group informally describes a slice of tests to be run in parallel
type Group []*Case

func (tg *Group) Run(t *testing.T) {
	t.Helper()
	// If the group contains only one test, no need to create a subtest
	sub := len(*tg) > 1
	// If we do have subtests, the root test is marked parallel
	if sub {
		t.Parallel()
	}
	// Run each subtest
	for _, tc := range *tg {
		tc.subIt = sub
		tc.Run(t)
	}
}

// Case describes an entire test-case, including data, setup and cleanup routines, command and expectations
type Case struct {
	// Description contains a human-readable short desc, used as a seed for the identifier and as a title for the test
	Description string
	// NoParallel disables parallel execution if set to true
	NoParallel bool
	// Env contains a map of environment variables to use for commands run in Setup, Command and Cleanup
	// Note that the environment is inherited by subtests
	Env map[string]string
	// Data contains test specific data, accessible to all operations, also inherited by subtests
	Data Data

	// Setup
	Setup Butler
	// Expected
	Expected Manager
	// Command
	Command Executor
	// Cleanup
	Cleanup Butler
	// Requirement
	Require *Requirement

	// SubTests
	SubTests []*Case

	// Private
	helpers     Helpers
	t           *testing.T
	parent      *Case
	baseCommand Command

	subIt bool
}

// Run prepares and executes the test, and any possible subtests
func (test *Case) Run(t *testing.T) {
	t.Helper()
	// Run the test
	testRun := func(subT *testing.T) {
		subT.Helper()

		assert.Assert(subT, test.t == nil, "You cannot run a test multiple times")

		// Attach testing.T
		test.t = subT
		assert.Assert(test.t, test.Description != "", "A test description cannot be empty")
		assert.Assert(test.t, test.Command == nil || test.Expected != nil,
			"Expectations for a test command cannot be nil. You may want to use Setup instead.")

		// Ensure we have env
		if test.Env == nil {
			test.Env = map[string]string{}
		}

		// If we have a parent, get parent env and data
		var parentData Data
		if test.parent != nil {
			parentData = test.parent.Data
			for k, v := range test.parent.Env {
				if _, ok := test.Env[k]; !ok {
					test.Env[k] = v
				}
			}
		}

		// Inherit and attach Data
		test.Data = configureData(test.t, test.Data, parentData)

		if registeredHooks == nil {
			bc := &GenericCommand{}
			bc.WithEnv(test.Env)
			bc.WithT(test.t)
			bc.WithTempDir(test.Data.TempDir())
			test.baseCommand = bc
		} else {
			test.baseCommand = registeredHooks.OnInitialize(test, test.t)
		}

		// Set base command
		test.helpers = &HelpersInternal{
			CmdInternal: test.baseCommand,
		}

		setups := []func(data Data, helpers Helpers){}
		cleanups := []func(data Data, helpers Helpers){}

		// Register custom cleanup if any - MUST run before Requirements cleanups
		if test.Cleanup != nil {
			cleanups = append(cleanups, test.Cleanup)
		}

		// Check the requirements before going any further
		if test.Require != nil {
			shouldRun, message := test.Require.Check(test.Data, test.helpers, test.t)
			if !shouldRun {
				test.t.Skipf("test skipped as: %s", message)
			} else {
				if test.Require.Setup != nil {
					setups = append(setups, test.Require.Setup)
				}
				if test.Require.Cleanup != nil {
					cleanups = append(cleanups, test.Require.Cleanup)
				}
			}
		}

		// Register setup if any
		if test.Setup != nil {
			setups = append(setups, test.Setup)
		}

		// Run optional post requirement hook
		if registeredHooks != nil {
			registeredHooks.OnPostRequirements(test, test.t, test.baseCommand)
		}

		// Set parallel unless asked not to
		if !test.NoParallel {
			test.t.Parallel()
		}

		// Execute cleanups now
		for _, cleanup := range cleanups {
			cleanup(test.Data, test.helpers)
		}
		test.t.Cleanup(func() {
			for _, cleanup := range cleanups {
				cleanup(test.Data, test.helpers)
			}
		})

		// Setup now
		for _, setup := range setups {
			setup(test.Data, test.helpers)
		}

		// ENV may have been changed by setup routines
		test.baseCommand.WithEnv(test.Env)
		// And config as well, which may have effects
		if registeredHooks != nil {
			registeredHooks.OnPostSetup(test, test.t, test.baseCommand)
		}

		// Run the command if any, with expectations
		// Note: if we have a command, we already know we DO have Expected
		if test.Command != nil {
			test.Command(test.Data, test.helpers).Run(test.Expected(test.Data, test.helpers))
		}

		// Go for the subtests now
		for _, subTest := range test.SubTests {
			subTest.parent = test
			subTest.subIt = true
			subTest.Run(test.t)
		}
	}

	if test.subIt {
		t.Run(test.Description, testRun)
	} else {
		testRun(t)
	}
}
