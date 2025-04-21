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

// DataLabels holds key-value test information set by the test authors.
// Note that retrieving a non-existent label will return the empty string.
type DataLabels interface {
	// Get returns the value of the requested label.
	Get(key string) string
	// Set will save the label `key` with `value`.
	Set(key, value string)
}

// DataTemp allows test authors to easily manipulate test fixtures / temporary files.
type DataTemp interface {
	// Load will retrieve the content stored in the file
	// Asserts on failure.
	Load(key ...string) string
	// Save will store the content in the file, ensuring parent dir exists, and return the path.
	// Asserts on failure.
	Save(data string, key ...string) string
	// SaveToWriter allows to write to the file as a writer.
	// This is particularly useful for encoding functions like pem.Encode.
	SaveToWriter(writer func(file io.Writer) error, key ...string) string
	// Path will return the absolute path for the asset, whether it exists or not.
	Path(key ...string) string
	// Exists asserts that the object exist.
	Exists(key ...string)
	// Dir ensures the directory under temp is created, and returns the path.
	// Asserts on failure.
	Dir(key ...string) string
}

// Data is meant to hold information about a test:
// - first, key-value data that the test implementer wants to carry around - this is Labels()
// - second, some commonly useful immutable test properties (eg: a way to generate unique
// identifiers for that test)
// - third, ability to manipulate test files inside managed temporary directories
// Note that Data Labels are inherited from parent test to subtest.
// This is not true for Identifier and Temp().Dir(), which are bound to the test itself, though temporary files
// can be accessed by subtests if their location is passed around (in Labels).
type Data interface {
	Temp() DataTemp
	Labels() DataLabels
	Identifier(suffix ...string) string
}

// Helpers provides a set of helpers to run commands with simple expectations,
// available at all stages of a test (Setup, Cleanup, etc...)
type Helpers interface {
	// Ensure runs a command and verifies it is succeeding.
	Ensure(args ...string)
	// Anyhow runs a command and ignores its result.
	Anyhow(args ...string)
	// Fail runs a command and verifies it failed.
	Fail(args ...string)
	// Capture runs a command, verifies it succeeded, and returns stdout.
	Capture(args ...string) string
	// Err runs a command, and returns stderr regardless of its outcome.
	// This is mostly useful for debugging.
	Err(args ...string) string

	// Command will return a populated command from the default internal command, with the provided
	// arguments, ready to be Run or further configured.
	Command(args ...string) TestableCommand
	// Custom will return a bare command, without configuration nor defaults (still has the Env).
	Custom(binary string, args ...string) TestableCommand

	// Read return the config value associated with a key.
	Read(key ConfigKey) ConfigValue
	// Write saves a value in the config.
	Write(key ConfigKey, value ConfigValue)

	// T returns the current testing object.
	T() *testing.T
}

// The TestableCommand interface represents a low-level command to execute, typically to be compared
// with an Expected. A TestableCommand can be used as a Case Command obviously, but also as part of
// a Setup or Cleanup routine, and as the basis of any type of helper.
// For more powerful use-cases outside of test cases, see below CustomizableCommand.
type TestableCommand interface {
	// WithBinary specifies what binary to execute.
	WithBinary(binary string)
	// WithArgs specifies the args to pass to the binary. Note that WithArgs can be used multiple
	// times and is additive.
	WithArgs(args ...string)
	// WithWrapper allows wrapping a command with another command (for example: `time`).
	WithWrapper(binary string, args ...string)
	// WithPseudoTTY will allocate a new pty and set the command stdin and stdout to it.
	WithPseudoTTY()
	// WithCwd allows specifying the working directory for the command.
	WithCwd(path string)
	// WithTimeout defines the execution timeout for a command.
	WithTimeout(timeout time.Duration)
	// Setenv allows to override a specific env variable directly for a specific command instead of test-wide
	Setenv(key, value string)
	// WithFeeder allows passing a reader to be fed to the command stdin.
	WithFeeder(fun func() io.Reader)
	// Feed allows passing a reader to be fed to the command stdin.
	Feed(r io.Reader)
	// Clone returns a copy of the command.
	Clone() TestableCommand

	// Run does execute the command, and compare the output with the provided expectation.
	// Passing nil for `Expected` will just run the command regardless of outcome.
	// An empty `&Expected{}` is (of course) equivalent to &Expected{Exit: 0}, meaning the command
	// is verified to be successful.
	Run(expect *Expected)
	// Background allows starting a command in the background.
	Background()
	// Signal sends a signal to a backgrounded command.
	Signal(sig os.Signal) error
	// Stderr allows retrieving the raw stderr output of the command once it has been run.
	Stderr() string
}

// Config is meant to hold information relevant to the binary (eg: flags defining certain behaviors,
// etc.)
type Config interface {
	Write(key ConfigKey, value ConfigValue) Config
	Read(key ConfigKey) ConfigValue
}
