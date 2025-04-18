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

//revive:disable:exported,package-comments,add-constant // TODO.
package test

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/internal"
	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/internal/com"
	"github.com/containerd/nerdctl/mod/tigron/internal/formatter"
)

const (
	defaultExecutionTimeout = 3 * time.Minute
	commandDecorator        = "⚙️"
	errorDecorator          = "🚫"
	exitDecorator           = "⚠️"
	stdoutDecorator         = "🟢"
	stderrDecorator         = "🟠"
	timeoutDecorator        = "⏰"
	cwdDecorator            = "📁"
	envDecorator            = "🌱"
	sigDecorator            = "⚡"
)

// CustomizableCommand is an interface meant for people who want to heavily customize the base
// command of their test case.
// FIXME: now that most of the logic got moved to the internal command, consider simplifying this /
// removing some of the extra layers from here
//

type CustomizableCommand interface {
	TestableCommand

	PrependArgs(args ...string)
	// WithBlacklist allows to filter out unwanted variables from the embedding environment -
	// default it pass any that is defined by WithEnv
	WithBlacklist(env []string)
	// T returns the current testing object
	T() *testing.T

	// withEnv *copies* the passed map to the environment of the command to be executed
	// Note that this will override any variable defined in the embedding environment
	withEnv(env map[string]string)
	// withTempDir specifies a temporary directory to use
	// FIXME: this is only required because of the current command extension mechanism
	withTempDir(path string)
	// WithConfig allows passing custom config properties from the test to the base command
	withConfig(config Config)
	withT(t *testing.T)
	// Clear does a clone, but will clear binary and arguments while retaining the env, or any other
	// custom properties Gotcha: if genericCommand is embedded with a custom Run and an overridden
	// clear to return the embedding type the result will be the embedding command, no longer the
	// genericCommand
	clear() TestableCommand

	// Will manipulate specific configuration option on the command
	// Note that config is a copy of the test config
	// Any modification done here will not be passed along to subtests, although they are shared
	// amongst all commands of the test.
	write(key ConfigKey, value ConfigValue)
	read(key ConfigKey) ConfigValue
}

func NewGenericCommand() CustomizableCommand {
	genericCom := &GenericCommand{
		Env: map[string]string{},
		cmd: &com.Command{},
	}

	genericCom.cmd.Env = genericCom.Env
	genericCom.cmd.Timeout = defaultExecutionTimeout

	return genericCom
}

// GenericCommand is a concrete Command implementation.
type GenericCommand struct {
	Config  Config
	TempDir string
	Env     map[string]string

	t *testing.T

	cmd   *com.Command
	async bool

	rawStdErr string
}

func (gc *GenericCommand) WithBinary(binary string) {
	gc.cmd.Binary = binary
}

func (gc *GenericCommand) WithArgs(args ...string) {
	gc.cmd.Args = append(gc.cmd.Args, args...)
}

func (gc *GenericCommand) WithWrapper(binary string, args ...string) {
	gc.cmd.WrapBinary = binary
	gc.cmd.WrapArgs = args
}

func (gc *GenericCommand) WithPseudoTTY() {
	gc.cmd.WithPTY(true, true, false)
}

func (gc *GenericCommand) Feed(r io.Reader) {
	gc.cmd.Feed(r)
}

func (gc *GenericCommand) Setenv(key, value string) {
	gc.cmd.Env[key] = value
}

func (gc *GenericCommand) WithFeeder(fun func() io.Reader) {
	gc.cmd.WithFeeder(fun)
}

func (gc *GenericCommand) WithCwd(path string) {
	gc.cmd.WorkingDir = path
}

func (gc *GenericCommand) WithBlacklist(env []string) {
	gc.cmd.EnvBlackList = env
}

func (gc *GenericCommand) WithWhitelist(env []string) {
	gc.cmd.EnvWhiteList = env
}

func (gc *GenericCommand) WithTimeout(timeout time.Duration) {
	gc.cmd.Timeout = timeout
}

func (gc *GenericCommand) PrependArgs(args ...string) {
	gc.cmd.PrependArgs = args
}

func (gc *GenericCommand) Background() {
	gc.async = true

	_ = gc.cmd.Run(context.WithValue(context.Background(), com.LoggerKey, gc.t))
}

func (gc *GenericCommand) Signal(sig os.Signal) error {
	//nolint:wrapcheck
	return gc.cmd.Signal(sig)
}

func (gc *GenericCommand) Run(expect *Expected) {
	gc.t.Helper()

	var debug [][]any

	if !gc.async {
		_ = gc.cmd.Run(context.WithValue(context.Background(), com.LoggerKey, gc.t))
	}

	debug = append(debug,
		[]any{"➡️", commandDecorator + " " + gc.cmd.Binary + " " + strings.Join(gc.cmd.Args, " ")},
	)

	// Wait for the command and if Err is not nil, append it to the debug information
	result, err := gc.cmd.Wait()
	if err != nil {
		debug = append(debug, []any{"", errorDecorator + " " + err.Error()})
	}

	// If we were requested to perform expectation matching, add non-empty debugging information
	if result != nil {
		gc.rawStdErr = result.Stderr

		if result.ExitCode != 0 {
			debug = append(debug, []any{"", exitDecorator + " " + strconv.Itoa(result.ExitCode)})
		}

		if result.Stdout != "" {
			debug = append(debug, []any{"", stdoutDecorator + " " + result.Stdout})
		}

		if result.Stderr != "" {
			debug = append(debug, []any{"", stderrDecorator + " " + result.Stderr})
		}

		if result.Signal != nil {
			debug = append(debug, []any{"", sigDecorator + " " + result.Signal.String()})
		}

		duration := result.Duration.String()
		if result.Duration < time.Second {
			duration = "<1s"
		}

		debug = append(debug,
			[]any{envDecorator, strings.Join(result.Environ, "\n")},
			[]any{timeoutDecorator, duration + " (limit: " + gc.cmd.Timeout.String() + ")"},
			[]any{cwdDecorator, gc.cmd.WorkingDir},
		)
	}

	// Print the command info
	gc.t.Log("\n\n" + formatter.Table(debug, "-") + "\n")

	// Now, check our expectations, if any
	if expect != nil {
		assertT := assertive.WithSilentSuccess(gc.t)

		// ExitCode goes first
		switch expect.ExitCode {
		case internal.ExitCodeNoCheck:
			// ExitCodeNoCheck means we do not care at all about what happened. Fire and forget...
		case internal.ExitCodeGenericFail:
			// ExitCodeGenericFail means we expect an error (excluding timeout, cancellation,
			// signalling).
			assertive.ErrorIs(
				assertT,
				err,
				com.ErrExecutionFailed,
				"Command should fail",
			)
		case internal.ExitCodeTimeout:
			assertive.ErrorIs(
				gc.t,
				err,
				com.ErrTimeout,
				"Command should time-out",
			)
		case internal.ExitCodeSignaled:
			assertive.ErrorIs(
				gc.t,
				err,
				com.ErrSignaled,
				"Command should get signaled",
			)
		case internal.ExitCodeSuccess:
			assertive.ErrorIsNil(
				assertT,
				err,
				"Command should succeed",
			)
		default:
			exc := -1
			if result != nil {
				exc = result.ExitCode
			}

			assertive.IsEqual(
				assertT,
				expect.ExitCode,
				exc,
				"Exit code should match expectation",
			)
		}

		// Switch to fail later mode so that we get ALL subsequent asserts failures from there on.
		// Also does allow using assert(ive) in go routines.
		assertT = assertive.WithFailLater(gc.t)

		// Range through the expected errors and confirm they are seen on stderr
		for _, expectErr := range expect.Errors {
			assertive.Contains(
				assertT,
				gc.rawStdErr,
				expectErr.Error(),
				"Stderr should contain expected error",
			)
		}

		// Finally, check the output if we are asked to
		// FIXME: we cannot pass on assertT further for now until we move to tig.T
		if expect.Output != nil {
			expect.Output(
				result.Stdout,
				"",
				gc.t,
			)
		}
	}
}

func (gc *GenericCommand) Stderr() string {
	return gc.rawStdErr
}

func (gc *GenericCommand) withEnv(env map[string]string) {
	for k, v := range env {
		gc.cmd.Env[k] = v
	}
}

func (gc *GenericCommand) withTempDir(path string) {
	gc.TempDir = path
}

func (gc *GenericCommand) withConfig(config Config) {
	gc.Config = config
}

func (gc *GenericCommand) Clone() TestableCommand {
	// Copy the command and return a new one - with almost everything from the parent command
	clone := *gc
	clone.rawStdErr = ""
	clone.async = false

	// Clone Env
	clone.Env = make(map[string]string, len(gc.Env))
	for k, v := range gc.Env {
		clone.Env[k] = v
	}

	// Clone the underlying command
	clone.cmd = gc.cmd.Clone()
	clone.cmd.Env = clone.Env

	return &clone
}

func (gc *GenericCommand) T() *testing.T {
	return gc.t
}

func (gc *GenericCommand) clear() TestableCommand {
	comcopy := *gc
	// Reset internal command
	comcopy.cmd = &com.Command{}
	comcopy.rawStdErr = ""
	comcopy.async = false
	// Clone Env
	comcopy.Env = make(map[string]string, len(gc.Env))
	// Reset configuration
	comcopy.Config = &config{}
	// Copy the env
	for k, v := range gc.Env {
		comcopy.Env[k] = v
	}

	comcopy.cmd.Env = comcopy.Env

	return &comcopy
}

func (gc *GenericCommand) withT(t *testing.T) {
	t.Helper()
	gc.t = t
}

func (gc *GenericCommand) read(key ConfigKey) ConfigValue {
	return gc.Config.Read(key)
}

func (gc *GenericCommand) write(key ConfigKey, value ConfigValue) {
	gc.Config.Write(key, value)
}
