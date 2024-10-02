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
	"io"
	"testing"
	"time"
)

// An Evaluator is a function that decides whether a test should run on not, to be fed to MakeRequirement
type Evaluator func(data Data, helpers Helpers, t *testing.T) (bool, string)

// A Requirement offers a way to verify random conditions to decide if a test should be skipped
// It can furthermore (optionally) provide custom setup and cleanup routines to perform
type Requirement struct {
	Check   Evaluator
	Setup   Butler
	Cleanup Butler
}

// A Butler is the function signature meant to be attached to a Setup or Cleanup routine for a test.Case
type Butler func(data Data, helpers Helpers)

// An Executor is the function signature meant to be attached to a test.Case Command
type Executor func(data Data, helpers Helpers) Command

// A Manager is the function signature to be run to produce expectations to be fed to a command
type Manager func(data Data, helpers Helpers) *Expected

// The Command interface represents a low-level command to execute, typically to be compared with an Expected
// A Command can be used as a Case Command obviously, but also as part of a Setup or Cleanup routine,
// and as the basis of any type of helper.
// A Command can be cloned, in which case, the subcommand inherits a copy of all of its Env and parameters.
// Typically, a Case has a base-command, from which all commands involved in the test are derived.
type Command interface {
	// WithBinary specifies what binary to execute
	WithBinary(binary string)
	// WithArgs specifies the args to pass to the binary. Note that WithArgs is additive.
	WithArgs(args ...string)
	// WithEnv adds the passed map to the environment of the command to be executed
	WithEnv(env map[string]string)
	// WithWrapper allows wrapping a command with another command (for example: `time`, `unbuffer`)
	WithWrapper(binary string, args ...string)
	// WithStdin allows passing a reader to be used for stdin for the command
	WithStdin(r io.Reader)
	// Run does execute the command, and compare the output with the provided expectation.
	// Passing nil for `Expected` will just run the command regardless of outcome.
	// An empty `&Expected{}` is (of course) equivalent to &Expected{Exit: 0}, meaning the command is verified to be
	// successful
	Run(expect *Expected)
	// Clone returns a copy of the command
	Clone() Command
	// Clear does a clone, but will clear binary and arguments, but retain the env, or any other custom properties
	// Gotcha: if GenericCommand is embedded with a custom Run and an overridden Clear to return the embedding type
	// the result will be the embedding command, no longer the GenericCommand
	Clear() Command
	// Background allows starting a command in the background
	Background(timeout time.Duration)
}

// A Comparator is the function signature to implement when implementing testing against stdout of a command
type Comparator func(stdout string, info string, t *testing.T)

// Expected expresses the expected output of a command
type Expected struct {
	// ExitCode to expect
	ExitCode int
	// Errors contains any error that (once serialized) should be seen in stderr
	Errors []error
	// Output function to match against stdout
	Output Comparator
}

type ConfigKey string
type ConfigValue string

type SystemKey string
type SystemValue string

// Data is meant to hold information about a test:
// - first, any random key value data that the test implementer wants to carry / modify - this is test data
// - second, configuration specific to the binary being tested - typically defined by the specialized command being tested
// - third, immutable "system" info (unique identifier, tempDir, or other SystemKey/Value pairs)
type Data interface {
	// Get returns the value of a certain key for custom data
	Get(key string) string
	// Set will save `value` for `key`
	Set(key string, value string) Data

	// Identifier returns the test identifier that can be used to name resources
	Identifier(suffix ...string) string
	// TempDir returns the test temporary directory
	TempDir() string
	// Sink allows to define ONCE a certain system property
	Sink(key SystemKey, value SystemValue)
	// Surface allows retrieving a certain system property
	Surface(key SystemKey) (SystemValue, error)

	// WithConfig allows setting a declared ConfigKey to a ConfigValue
	WithConfig(key ConfigKey, value ConfigValue) Data
	ReadConfig(key ConfigKey) ConfigValue

	// Private methods
	getLabels() map[string]string
	getConfig() map[ConfigKey]ConfigValue
}

type Hooks interface {
	OnInitialize(testCase *Case, t *testing.T) Command
	OnPostRequirements(testCase *Case, t *testing.T, com Command)
	OnPostSetup(testCase *Case, t *testing.T, com Command)
}

var (
	registeredHooks Hooks
)

func CustomCommand(hooks Hooks) {
	registeredHooks = hooks
}
