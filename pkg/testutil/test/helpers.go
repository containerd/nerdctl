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

// Helpers provides a set of helpers to run commands with simple expectations, available in most stages of a test (Setup, Cleanup, etc...)
type Helpers interface {
	Ensure(args ...string)
	Anyhow(args ...string)
	Fail(args ...string)
	Capture(args ...string) string

	Command(args ...string) Command
	CustomCommand(binary string, args ...string) Command
}

// FIXME: see `nerdtest/test.go` for why this is exported - it should not be

type HelpersInternal struct {
	CmdInternal Command
}

// Ensure will run a command and make sure it is successful
func (hel *HelpersInternal) Ensure(args ...string) {
	hel.Command(args...).Run(&Expected{
		ExitCode: 0,
	})
}

// Anyhow will run a command regardless of outcome (may or may not fail)
func (hel *HelpersInternal) Anyhow(args ...string) {
	hel.Command(args...).Run(nil)
}

// Fail will run a command and make sure it does fail
func (hel *HelpersInternal) Fail(args ...string) {
	hel.Command(args...).Run(&Expected{
		ExitCode: 1,
	})
}

// Capture will run a command, ensure it is successful and return stdout
func (hel *HelpersInternal) Capture(args ...string) string {
	var ret string
	hel.Command(args...).Run(&Expected{
		Output: func(stdout string, info string, t *testing.T) {
			ret = stdout
		},
	})
	return ret
}

// Command will return a clone of your base command without running it
func (hel *HelpersInternal) Command(args ...string) Command {
	cc := hel.CmdInternal.Clone()
	cc.WithArgs(args...)
	return cc
}

// CustomCommand will return a command for the requested binary and args, with all the environment of your test
// (eg: Env, Cwd, etc.)
func (hel *HelpersInternal) CustomCommand(binary string, args ...string) Command {
	cc := hel.CmdInternal.Clear()
	cc.WithBinary(binary)
	cc.WithArgs(args...)
	return cc
}
