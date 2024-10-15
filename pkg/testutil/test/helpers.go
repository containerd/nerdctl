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

import "testing"

// Helpers provides a set of helpers to run commands with simple expectations, available at all stages of a test (Setup, Cleanup, etc...)
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

// This is the implementation of Helpers

type helpersInternal struct {
	cmdInternal CustomizableCommand

	t *testing.T
}

// Ensure will run a command and make sure it is successful
func (help *helpersInternal) Ensure(args ...string) {
	help.Command(args...).Run(&Expected{
		ExitCode: 0,
	})
}

// Anyhow will run a command regardless of outcome (may or may not fail)
func (help *helpersInternal) Anyhow(args ...string) {
	help.Command(args...).Run(nil)
}

// Fail will run a command and make sure it does fail
func (help *helpersInternal) Fail(args ...string) {
	help.Command(args...).Run(&Expected{
		ExitCode: -1,
	})
}

// Capture will run a command, ensure it is successful and return stdout
func (help *helpersInternal) Capture(args ...string) string {
	var ret string
	help.Command(args...).Run(&Expected{
		Output: func(stdout string, info string, t *testing.T) {
			ret = stdout
		},
	})
	return ret
}

// Capture will run a command, ensure it is successful and return stdout
func (help *helpersInternal) Err(args ...string) string {
	cmd := help.Command(args...)
	cmd.Run(nil)
	return cmd.Stderr()
}

// Command will return a clone of your base command without running it
func (help *helpersInternal) Command(args ...string) TestableCommand {
	cc := help.cmdInternal.Clone()
	cc.WithArgs(args...)
	return cc
}

// Custom will return a command for the requested binary and args, with the environment of your test
// (eg: Env, Cwd, etc.)
func (help *helpersInternal) Custom(binary string, args ...string) TestableCommand {
	cc := help.cmdInternal.clear()
	cc.WithBinary(binary)
	cc.WithArgs(args...)
	return cc
}

func (help *helpersInternal) Read(key ConfigKey) ConfigValue {
	return help.cmdInternal.read(key)
}

func (help *helpersInternal) Write(key ConfigKey, value ConfigValue) {
	help.cmdInternal.write(key, value)
}

func (help *helpersInternal) T() *testing.T {
	return help.t
}
