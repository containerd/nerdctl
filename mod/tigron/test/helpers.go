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

	"github.com/containerd/nerdctl/mod/tigron/internal"
)

// This is the implementation of Helpers

type helpersInternal struct {
	cmdInternal CustomizableCommand

	t *testing.T
}

// Ensure will run a command and make sure it is successful.
func (help *helpersInternal) Ensure(args ...string) {
	help.t.Helper()
	help.Command(args...).Run(&Expected{
		ExitCode: internal.ExitCodeSuccess,
	})
}

// Anyhow will run a command regardless of outcome (may or may not fail).
func (help *helpersInternal) Anyhow(args ...string) {
	help.t.Helper()
	help.Command(args...).Run(&Expected{
		ExitCode: internal.ExitCodeNoCheck,
	})
}

// Fail will run a command and make sure it does fail.
func (help *helpersInternal) Fail(args ...string) {
	help.t.Helper()
	help.Command(args...).Run(&Expected{
		ExitCode: internal.ExitCodeGenericFail,
	})
}

// Capture will run a command, ensure it is successful and return stdout.
func (help *helpersInternal) Capture(args ...string) string {
	var ret string

	help.t.Helper()
	help.Command(args...).Run(&Expected{
		//nolint:thelper
		Output: func(stdout, _ string, _ *testing.T) {
			ret = stdout
		},
	})

	return ret
}

// Err will run a command with no expectation and return Stderr.
func (help *helpersInternal) Err(args ...string) string {
	help.t.Helper()
	cmd := help.Command(args...)
	cmd.Run(nil)

	return cmd.Stderr()
}

// Command will return a clone of your base command without running it.
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
