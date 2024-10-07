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

// A Requirement offers a way to verify random conditions to decide if a test should be skipped or run.
// It can furthermore (optionally) provide custom Setup and Cleanup routines.
type Requirement struct {
	// Check is expected to perform random operations and return a boolean and an explanatory message
	Check Evaluator
	// Setup, if provided, will be run before any test-specific Setup routine, in the order that requirements have been declared
	Setup Butler
	// Cleanup, if provided, will be run after any test-specific Cleanup routine, in the revers order that requirements have been declared
	Cleanup Butler
}

// An Evaluator is a function that decides whether a test should run or not
type Evaluator func(data Data, helpers Helpers) (bool, string)

// A Butler is the function signature meant to be attached to a Setup or Cleanup routine for a Case or Requirement
type Butler func(data Data, helpers Helpers)

// An Executor is the function signature meant to be attached to the Command property of a Case
type Executor func(data Data, helpers Helpers) TestableCommand

// A Manager is the function signature to be run to produce expectations to be fed to a command
type Manager func(data Data, helpers Helpers) *Expected

// A Comparator is the function signature to implement for the Output property of an Expected
type Comparator func(stdout string, info string, t *testing.T)

// Expected expresses the expected output of a command
type Expected struct {
	// ExitCode
	ExitCode int
	// Errors contains any error that (once serialized) should be seen in stderr
	Errors []error
	// Output function to match against stdout
	Output Comparator
}

// Data is meant to hold information about a test:
// - first, any random key value data that the test implementer wants to carry / modify - this is test data
// - second, some commonly useful immutable test properties (a way to generate unique identifiers for that test,
// temporary directory, etc.)
// Note that Data is inherited, from parent test to subtest (except for Identifier and TempDir of course)
type Data interface {
	// Get returns the value of a certain key for custom data
	Get(key string) string
	// Set will save `value` for `key`
	Set(key string, value string) Data

	// Identifier returns the test identifier that can be used to name resources
	Identifier(suffix ...string) string
	// TempDir returns the test temporary directory
	TempDir() string
}

type ConfigKey string
type ConfigValue string

// Config is meant to hold information relevant to the binary (eg: flags defining certain behaviors, etc.)
type Config interface {
	// Write
	Write(key ConfigKey, value ConfigValue) Config
	// Read
	Read(key ConfigKey) ConfigValue
}

// The TestableCommand interface represents a low-level command to execute, typically to be compared with an Expected
// A TestableCommand can be used as a Case Command obviously, but also as part of a Setup or Cleanup routine,
// and as the basis of any type of helper.
// For more powerful usecase outside of test cases, see below CustomizableCommand
type TestableCommand interface {
	// WithBinary specifies what binary to execute
	WithBinary(binary string)
	// WithArgs specifies the args to pass to the binary. Note that WithArgs can be used multiple times and is additive.
	WithArgs(args ...string)
	// WithWrapper allows wrapping a command with another command (for example: `time`, `unbuffer`)
	WithWrapper(binary string, args ...string)
	// WithStdin allows passing a reader to be used for stdin for the command
	WithStdin(r io.Reader)
	// WithCwd allows specifying the working directory for the command
	WithCwd(path string)
	// Clone returns a copy of the command
	Clone() TestableCommand

	// Run does execute the command, and compare the output with the provided expectation.
	// Passing nil for `Expected` will just run the command regardless of outcome.
	// An empty `&Expected{}` is (of course) equivalent to &Expected{Exit: 0}, meaning the command is verified to be
	// successful
	Run(expect *Expected)
	// Background allows starting a command in the background
	Background(timeout time.Duration)
	// Stderr allows retrieving the raw stderr output of the command
	Stderr() string
}

// /////////////////////////////////////////////
// CustomizableCommand is an interface meant for people who want to heavily customize the base command of their test case
// It is passed along
type CustomizableCommand interface {
	TestableCommand

	PrependArgs(args ...string)
	// WithBlacklist allows to filter out unwanted variables from the embedding environment - default it pass any that is
	// defined by WithEnv
	WithBlacklist(env []string)

	// withEnv *copies* the passed map to the environment of the command to be executed
	// Note that this will override any variable defined in the embedding environment
	withEnv(env map[string]string)
	// withTempDir specifies a temporary directory to use
	withTempDir(path string)
	// WithConfig allows passing custom config properties from the test to the base command
	withConfig(config Config)
	withT(t *testing.T)
	// Clear does a clone, but will clear binary and arguments, but retain the env, or any other custom properties
	// Gotcha: if GenericCommand is embedded with a custom Run and an overridden clear to return the embedding type
	// the result will be the embedding command, no longer the GenericCommand
	clear() TestableCommand

	// Will manipulate specific configuration option on the command
	// Note that config is a copy of the test config
	// Any modification done here will not be passed along to subtests, although they are shared amongst all commands of the test.
	write(key ConfigKey, value ConfigValue)
	read(key ConfigKey) ConfigValue
}

type Testable interface {
	CustomCommand(testCase *Case, t *testing.T) CustomizableCommand
	AmbientRequirements(testCase *Case, t *testing.T)
}

var (
	registeredTestable Testable
)

func Customize(testable Testable) {
	registeredTestable = testable
}
