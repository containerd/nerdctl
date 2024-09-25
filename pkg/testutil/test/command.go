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
	WorkingDir string
	Env        map[string]string

	t            *testing.T
	tempDir      string
	helperBinary string
	helperArgs   []string
	mainBinary   string
	mainArgs     []string
	result       *icmd.Result
	stdin        io.Reader
	async        bool
	timeout      time.Duration
}

func (gc *GenericCommand) WithBinary(binary string) Command {
	gc.mainBinary = binary
	return gc
}

func (gc *GenericCommand) WithArgs(args ...string) Command {
	gc.mainArgs = append(gc.mainArgs, args...)
	return gc
}

// WithEnv will overload the command env with values from the passed map
func (gc *GenericCommand) WithEnv(env map[string]string) Command {
	if gc.Env == nil {
		gc.Env = map[string]string{}
	}
	for k, v := range env {
		gc.Env[k] = v
	}
	return gc
}

func (gc *GenericCommand) WithWrapper(binary string, args ...string) Command {
	gc.helperBinary = binary
	gc.helperArgs = args
	return gc
}

// WithStdin sets the standard input of Cmd to the specified reader
func (gc *GenericCommand) WithStdin(r io.Reader) Command {
	gc.stdin = r
	return gc
}

func (gc *GenericCommand) Background(timeout time.Duration) Command {
	// Run it
	gc.async = true
	i := gc.boot()
	gc.result = icmd.StartCmd(i)
	gc.timeout = timeout
	return gc
}

// TODO: it should be possible to:
// - timeout execution
func (gc *GenericCommand) Run(expect *Expected) {
	var result *icmd.Result
	var env []string
	if gc.async {
		result = icmd.WaitOnCmd(gc.timeout, gc.result)
		env = gc.result.Cmd.Env
	} else {
		icmdCmd := gc.boot()
		env = icmdCmd.Env
		// Run it
		result = icmd.RunCmd(icmdCmd)
	}

	// Check our expectations, if any
	if expect != nil {
		// Build the debug string - additionally attach the env (which icmd does not do)
		debug := result.String() + "Env:\n" + strings.Join(env, "\n")
		// ExitCode goes first
		if expect.ExitCode == -1 {
			assert.Assert(gc.t, result.ExitCode != 0,
				"Expected exit code to be different than 0"+debug)
		} else {
			assert.Assert(gc.t, expect.ExitCode == result.ExitCode,
				fmt.Sprintf("Expected exit code: %d", expect.ExitCode)+debug)
		}
		// Range through the expected errors and confirm they are seen on stderr
		for _, expectErr := range expect.Errors {
			assert.Assert(gc.t, strings.Contains(result.Stderr(), expectErr.Error()),
				fmt.Sprintf("Expected error: %q to be found in stderr", expectErr.Error())+debug)
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
	args := gc.mainArgs
	if gc.helperBinary != "" {
		args = append([]string{binary}, args...)
		args = append(gc.helperArgs, args...)
		binary = gc.helperBinary
	}

	// Create the command and set the env
	// TODO: do we really need icmd?
	icmdCmd := icmd.Command(binary, args...)
	icmdCmd.Env = []string{}
	for _, v := range os.Environ() {
		// Ignore LS_COLORS from the env, just too much noise
		if !strings.HasPrefix(v, "LS_COLORS") {
			icmdCmd.Env = append(icmdCmd.Env, v)
		}
	}

	// Ensure the subprocess gets executed in a temporary directory unless explicitly instructed otherwise
	icmdCmd.Dir = gc.WorkingDir
	if icmdCmd.Dir == "" {
		icmdCmd.Dir = gc.tempDir
	}

	if gc.stdin != nil {
		icmdCmd.Stdin = gc.stdin
	}

	// Attach any extra env we have
	for k, v := range gc.Env {
		icmdCmd.Env = append(icmdCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return icmdCmd
}

func (gc *GenericCommand) Clone() Command {
	// Copy the command and return a new one - with WorkingDir, binary, args, etc
	cc := *gc
	// Clone Env
	cc.Env = make(map[string]string, len(gc.Env))
	for k, v := range gc.Env {
		cc.Env[k] = v
	}
	return &cc
}

func (gc *GenericCommand) Clear() Command {
	gc.mainBinary = ""
	gc.helperBinary = ""
	gc.mainArgs = []string{}
	gc.helperArgs = []string{}
	return gc
}

func (gc *GenericCommand) WithT(t *testing.T) Command {
	gc.t = t
	return gc
}

func (gc *GenericCommand) WithTempDir(tempDir string) {
	gc.tempDir = tempDir
}
