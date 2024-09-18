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

type Helpers interface {
	Ensure(args ...string)
	Anyhow(args ...string)
	Fail(args ...string)
	Capture(args ...string) string

	Command(args ...string) Command
	CustomCommand(binary string, args ...string) Command
}

type helpers struct {
	cmd Command
}

func (hel *helpers) Ensure(args ...string) {
	hel.Command(args...).Run(&Expected{})
}

func (hel *helpers) Anyhow(args ...string) {
	hel.Command(args...).Run(nil)
}

func (hel *helpers) Fail(args ...string) {
	hel.Command(args...).Run(&Expected{
		ExitCode: 1,
	})
}

func (hel *helpers) Capture(args ...string) string {
	var ret string
	hel.Command(args...).Run(&Expected{
		Output: func(stdout string, info string, t *testing.T) {
			ret = stdout
		},
	})
	return ret
}

func (hel *helpers) Command(args ...string) Command {
	cc := hel.cmd.Clone()
	cc.WithArgs(args...)
	return cc
}

func (hel *helpers) CustomCommand(binary string, args ...string) Command {
	cc := hel.cmd.Clone()
	cc.Clear()
	cc.WithBinary(binary)
	cc.WithArgs(args...)
	return cc
}
