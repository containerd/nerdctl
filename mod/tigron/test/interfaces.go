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
	"os"
	"testing"
	"time"
)

// Data is meant to hold information about a test:
// - first, any random key value data that the test implementer wants to carry / modify - this is test data
// - second, some commonly useful immutable test properties (a way to generate unique identifiers for that test,
// temporary directory, etc.)
// Note that Data is inherited, from parent test to subtest (except for Identifier and TempDir of course).
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

// Helpers provides a set of helpers to run commands with simple expectations,
// available at all stages of a test (Setup, Cleanup, etc...)
type Helpers interface {
	// Ensure runs a command and verifies it is succeeding
	Ensure(args ...string)
	// Anyhow runs a command and ignores its result
	Anyhow(args ...string)
	// Fail runs a command and verifies it failed
	Fail(args ...string)
	// Capture runs a command, verifies it succeeded, and returns stdout
	Capture(args ...string) string
	// Err runs a command, and returns stderr regardless of its outcome
	// This is mostly useful for debugging
	Err(args ...string) string

	// Command will return a populated command from the default internal command, with the provided arguments,
	// ready to be Run or further configured
	Command(args ...string) TestableCommand
	// Custom will return a bare command, without configuration nor defaults (still has the Env)
	Custom(binary string, args ...string) TestableCommand

	// Read return the config value associated with a key
	Read(key ConfigKey) ConfigValue
	// Write saves a value in the config
	Write(key ConfigKey, value ConfigValue)

	// T returns the current testing object
	T() *testing.T
}

// The TestableCommand interface represents a low-level command to execute, typically to be compared with an Expected
// A TestableCommand can be used as a Case Command obviously, but also as part of a Setup or Cleanup routine,
// and as the basis of any type of helper.
// For more powerful use-cases outside of test cases, see below CustomizableCommand.
type TestableCommand interface { //nolint:interfacebloat
	// WithBinary specifies what binary to execute
	WithBinary(binary string)
	// WithArgs specifies the args to pass to the binary. Note that WithArgs can be used multiple times and is additive.
	WithArgs(args ...string)
	// WithWrapper allows wrapping a command with another command (for example: `time`)
	WithWrapper(binary string, args ...string)
	WithPseudoTTY(writers ...func(*os.File) error)
	// WithStdin allows passing a reader to be used for stdin for the command
	WithStdin(r io.Reader)
	// WithCwd allows specifying the working directory for the command
	WithCwd(path string)
	// Clone returns a copy of the command
	Clone() TestableCommand

	// Run does execute the command, and compare the output with the provided expectation.
	// Passing nil for `Expected` will just run the command regardless of outcome.
	// An empty `&Expected{}` is (of course) equivalent to &Expected{Exit: 0}, meaning the command is
	// verified to be successful
	Run(expect *Expected)
	// Background allows starting a command in the background
	Background(timeout time.Duration)
	// Signal sends a signal to a backgrounded command
	Signal(sig os.Signal) error
	// Stderr allows retrieving the raw stderr output of the command once it has been run
	Stderr() string
}

// Config is meant to hold information relevant to the binary (eg: flags defining certain behaviors, etc.)
type Config interface {
	Write(key ConfigKey, value ConfigValue) Config
	Read(key ConfigKey) ConfigValue
}
