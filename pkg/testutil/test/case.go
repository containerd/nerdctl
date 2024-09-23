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

// Group informally describes a slice of tests
type Group []*Case

func (tg *Group) Run(t *testing.T) {
	t.Helper()
	// If the group contains only one test, no need to create a subtest
	sub := len(*tg) > 1
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
	Require Requirement

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
	testRun := func(tt *testing.T) {
		tt.Helper()
		test.seal(tt)

		if registeredInit == nil {
			bc := &GenericCommand{}
			bc.WithEnv(test.Env)
			bc.WithT(tt)
			bc.WithTempDir(test.Data.TempDir())
			test.baseCommand = bc
		} else {
			test.baseCommand = registeredInit(test, test.t)
		}

		test.exec(tt)
	}

	if test.subIt {
		t.Run(test.Description, testRun)
	} else {
		testRun(t)
	}
}

// seal is a private method to prepare the test
func (test *Case) seal(t *testing.T) {
	t.Helper()
	assert.Assert(t, test.t == nil, "You cannot run a test multiple times")
	assert.Assert(t, test.Description != "", "A test description cannot be empty")
	assert.Assert(t, test.Command == nil || test.Expected != nil,
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

	// Attach testing.T
	test.t = t
	// Inherit and attach Data
	test.Data = configureData(t, test.Data, parentData)

	// Check the requirements
	if test.Require != nil {
		test.Require(test.Data, t)
	}
}

// exec is a private method that will take care of the test setup, command and cleanup execution
func (test *Case) exec(t *testing.T) {
	t.Helper()
	test.helpers = &helpers{
		test.baseCommand,
	}

	// Set parallel unless asked not to
	if !test.NoParallel {
		t.Parallel()
	}

	// Register cleanup if there is any, and run it to collect any leftovers from previous runs
	if test.Cleanup != nil {
		test.Cleanup(test.Data, test.helpers)
		t.Cleanup(func() {
			test.Cleanup(test.Data, test.helpers)
		})
	}

	// Run setup
	if test.Setup != nil {
		test.Setup(test.Data, test.helpers)
	}

	// Run the command if any, with expectations
	if test.Command != nil {
		test.Command(test.Data, test.helpers).Run(test.Expected(test.Data, test.helpers))
	}

	for _, subTest := range test.SubTests {
		subTest.parent = test
		subTest.subIt = true
		subTest.Run(t)
	}
}
