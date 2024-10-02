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
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

// GenericCommand is a concrete Command implementation
type GenericCommand struct {
	WorkingDir   string
	Env          map[string]string
	EnvBlackList []string

	t            *testing.T
	tempDir      string
	helperBinary string
	helperArgs   []string
	prependArgs  []string
	mainBinary   string
	mainArgs     []string
	result       *icmd.Result

	stdin   io.Reader
	async   bool
	timeout time.Duration
}

func (gc *GenericCommand) WithBinary(binary string) {
	gc.mainBinary = binary
}

func (gc *GenericCommand) WithArgs(args ...string) {
	gc.mainArgs = append(gc.mainArgs, args...)
}

func (gc *GenericCommand) PrependArgs(args ...string) {
	gc.prependArgs = append(gc.prependArgs, args...)
}

// WithEnv will overload the command env with values from the passed map
func (gc *GenericCommand) WithEnv(env map[string]string) {
	if gc.Env == nil {
		gc.Env = map[string]string{}
	}
	for k, v := range env {
		gc.Env[k] = v
	}
}

func (gc *GenericCommand) WithWrapper(binary string, args ...string) {
	gc.helperBinary = binary
	gc.helperArgs = args
}

// WithStdin sets the standard input of Cmd to the specified reader
func (gc *GenericCommand) WithStdin(r io.Reader) {
	gc.stdin = r
}

func (gc *GenericCommand) Background(timeout time.Duration) {
	// Run it
	gc.async = true
	i := gc.boot()
	gc.timeout = timeout
	gc.result = icmd.StartCmd(i)
}

// TODO: it should be possible to timeout execution
// Primitives (gc.timeout) is here, it is just a matter of exposing a WithTimeout method
// - UX to be decided
// - validate use case: would we ever need this?

func (gc *GenericCommand) Run(expect *Expected) {
	if gc.t != nil {
		gc.t.Helper()
	}

	var result *icmd.Result
	var env []string
	if gc.async {
		result = icmd.WaitOnCmd(gc.timeout, gc.result)
		env = gc.result.Cmd.Env
	} else {
		iCmdCmd := gc.boot()
		env = iCmdCmd.Env
		// Run it
		result = icmd.RunCmd(iCmdCmd)
	}

	// Check our expectations, if any
	if expect != nil {
		// Build the debug string - additionally attach the env (which iCmd does not do)
		debug := result.String() + "Env:\n" + strings.Join(env, "\n")
		// ExitCode goes first
		if expect.ExitCode == -1 {
			assert.Assert(gc.t, result.ExitCode != 0,
				"Expected exit code to be different than 0\n"+debug)
		} else {
			assert.Assert(gc.t, expect.ExitCode == result.ExitCode,
				fmt.Sprintf("Expected exit code: %d\n", expect.ExitCode)+debug)
		}
		// Range through the expected errors and confirm they are seen on stderr
		for _, expectErr := range expect.Errors {
			assert.Assert(gc.t, strings.Contains(result.Stderr(), expectErr.Error()),
				fmt.Sprintf("Expected error: %q to be found in stderr\n", expectErr.Error())+debug)
		}
		// Finally, check the output if we are asked to
		if expect.Output != nil {
			expect.Output(result.Stdout(), debug, gc.t)
		}
	}
}

func (gc *GenericCommand) boot() icmd.Cmd {
	// This is a helper function, not to appear in the debugging output
	if gc.t != nil {
		gc.t.Helper()
	}

	binary := gc.mainBinary
	args := append(gc.prependArgs, gc.mainArgs...)
	if gc.helperBinary != "" {
		args = append([]string{binary}, args...)
		args = append(gc.helperArgs, args...)
		binary = gc.helperBinary
	}

	// Create the command and set the env
	// TODO: do we really need iCmd?
	gc.t.Log(binary, strings.Join(args, " "))

	iCmdCmd := icmd.Command(binary, args...)
	iCmdCmd.Env = []string{}
	for _, v := range os.Environ() {
		add := true
		for _, b := range gc.EnvBlackList {
			if strings.HasPrefix(v, b+"=") {
				add = false
				break
			}
		}
		if add {
			iCmdCmd.Env = append(iCmdCmd.Env, v)
		}
	}

	// Ensure the subprocess gets executed in a temporary directory unless explicitly instructed otherwise
	iCmdCmd.Dir = gc.WorkingDir
	if iCmdCmd.Dir == "" {
		iCmdCmd.Dir = gc.tempDir
	}

	if gc.stdin != nil {
		iCmdCmd.Stdin = gc.stdin
	}

	// Attach any extra env we have
	for k, v := range gc.Env {
		iCmdCmd.Env = append(iCmdCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return iCmdCmd
}

func (gc *GenericCommand) Clone() Command {
	// Copy the command and return a new one - with almost everything from the parent command
	cc := *gc
	cc.result = nil
	cc.stdin = nil
	cc.timeout = 0
	// Clone Env
	cc.Env = make(map[string]string, len(gc.Env))
	for k, v := range gc.Env {
		cc.Env[k] = v
	}
	return &cc
}

func (gc *GenericCommand) Clear() Command {
	cc := *gc
	cc.mainBinary = ""
	cc.helperBinary = ""
	cc.mainArgs = []string{}
	cc.prependArgs = []string{}
	cc.helperArgs = []string{}
	// Clone Env
	cc.Env = make(map[string]string, len(gc.Env))
	for k, v := range gc.Env {
		cc.Env[k] = v
	}
	return &cc
}

func (gc *GenericCommand) WithT(t *testing.T) Command {
	gc.t = t
	return gc
}

func (gc *GenericCommand) WithTempDir(tempDir string) {
	gc.tempDir = tempDir
}

func (gc *GenericCommand) T() *testing.T {
	return gc.t
}

func (gc *GenericCommand) TempDir() string {
	return gc.tempDir
}
