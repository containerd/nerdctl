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

//revive:disable:add-constant
package com_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/internal/com"
)

const windows = "windows"

// Testing faulty code (double run, etc.)

func TestFaultyDoubleRunWait(t *testing.T) {
	// Double run returns an error on the second run, but Wait will still work properly
	t.Parallel()

	command := &com.Command{
		Binary:  "printf",
		Args:    []string{"one"},
		Timeout: time.Second,
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	err = command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIs(t, err, com.ErrExecAlreadyStarted, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, expect.ExitCodeSuccess, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "one", res.Stdout, "Stdout")
	assertive.IsEqual(t, "", res.Stderr, "Stderr")
}

func TestFaultyRunDoubleWait(t *testing.T) {
	// Double wait returns an error on the second wait, but also returns the existing result
	t.Parallel()

	command := &com.Command{
		Binary:  "printf",
		Args:    []string{"one"},
		Timeout: time.Second,
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, expect.ExitCodeSuccess, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "one", res.Stdout, "Stdout")
	assertive.IsEqual(t, "", res.Stderr, "Stderr")

	res, err = command.Wait()

	assertive.ErrorIs(t, err, com.ErrExecAlreadyFinished, "Err")
	assertive.IsEqual(t, expect.ExitCodeSuccess, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "one", res.Stdout, "Stdout")
	assertive.IsEqual(t, "", res.Stderr, "Stderr")
}

func TestFailRun(t *testing.T) {
	t.Parallel()

	command := &com.Command{
		Binary: "does-not-exist",
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIs(t, err, com.ErrFailedStarting, "Err")

	err = command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIs(t, err, com.ErrExecAlreadyFinished, "Err")

	res, err := command.Wait()

	assertive.ErrorIs(t, err, com.ErrFailedStarting, "Err")
	assertive.IsEqual(t, -1, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "", res.Stdout, "Stdout")
	assertive.IsEqual(t, "", res.Stderr, "Stderr")

	res, err = command.Wait()

	assertive.ErrorIs(t, err, com.ErrFailedStarting, "Err")
	assertive.IsEqual(t, -1, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "", res.Stdout, "Stdout")
	assertive.IsEqual(t, "", res.Stderr, "Stderr")
}

func TestBasicRunWait(t *testing.T) {
	t.Parallel()

	command := &com.Command{
		Binary: "printf",
		Args:   []string{"one"},
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, 0, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "one", res.Stdout, "Stdout")
	assertive.IsEqual(t, "", res.Stderr, "Stderr")
}

func TestBasicFail(t *testing.T) {
	t.Parallel()

	command := &com.Command{
		Binary: "bash",
		Args:   []string{"-c", "--", "does-not-exist"},
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIs(t, err, com.ErrExecutionFailed, "Err")
	assertive.IsEqual(t, 127, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "", res.Stdout, "Stdout")
	assertive.HasSuffix(t, res.Stderr, "does-not-exist: command not found\n", "Stderr")
}

func TestWorkingDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	command := &com.Command{
		Binary:     "pwd",
		WorkingDir: dir,
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, 0, res.ExitCode, "ExitCode")

	// Note:
	// - darwin will link to /private/DIR, so, check with HasSuffix
	// - windows+ming will go to C:\Users\RUNNER~1\AppData\Local\Temp\, so, ignore Windows
	if runtime.GOOS == windows {
		t.Skip("skipping last check on windows, see note")
	}

	assertive.HasSuffix(t, res.Stdout, dir+"\n", "Stdout")
}

func TestEnvBlacklist(t *testing.T) {
	t.Setenv("FOO", "BAR")
	t.Setenv("FOOBAR", "BARBAR")

	// First, test that environment gets through to the command
	command := &com.Command{
		Binary: "env",
		// Note: LS_COLORS is just too loud
		EnvBlackList: []string{
			"LS_COLORS",
		},
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, 0, res.ExitCode, "ExitCode")
	assertive.Contains(t, res.Stdout, "FOO=BAR", "Stdout")
	assertive.Contains(t, res.Stdout, "FOOBAR=BARBAR", "Stdout")

	// Now test that we can blacklist a single variable with fully qualified name (FOO)
	command = &com.Command{
		Binary: "env",
		EnvBlackList: []string{
			"LS_COLORS",
			"FOO",
		},
	}

	err = command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err = command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	assertive.DoesNotContain(t, res.Stdout, "FOO=", "Stdout")
	assertive.Contains(t, res.Stdout, "FOOBAR=BARBAR", "Stdout")

	// Now test that we can blacklist multiple variables with FOO*
	command = &com.Command{
		Binary: "env",
		EnvBlackList: []string{
			"LS_COLORS",
			"FOO*",
		},
	}

	err = command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err = command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	assertive.DoesNotContain(t, res.Stdout, "FOO=", "Stdout")
	assertive.DoesNotContain(t, res.Stdout, "FOOBAR=", "Stdout")

	// On windows, with mingw, SYSTEMROOT,TERM and HOME (possibly others) will be forcefully added
	// to the environment regardless, so, we can't test "*" blacklist
	if runtime.GOOS == windows {
		t.Skip(
			"Windows/mingw will always repopulate the environment with extra variables we cannot bypass",
		)
	}

	// Now, test that we can blacklist everything
	command = &com.Command{
		Binary:       "env",
		EnvBlackList: []string{"*"},
	}

	err = command.Run(context.WithValue(context.Background(), com.LoggerKey, t))
	assertive.ErrorIsNil(t, err, "Err")

	res, err = command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	assertive.IsEqual(t, res.Stdout, "", "Stdout")
}

func TestEnvWhiteList(t *testing.T) {
	t.Setenv("FOO", "BAR")
	t.Setenv("FOOBAR", "BARBAR")

	// Test that whitelist does let through only FOO
	command := &com.Command{
		Binary: "env",
		EnvWhiteList: []string{
			"FOO",
		},
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, 0, res.ExitCode, "ExitCode")
	assertive.Contains(t, res.Stdout, "FOO=BAR", "Stdout")
	assertive.DoesNotContain(t, res.Stdout, "FOOBAR=", "Stdout")
	assertive.DoesNotContain(t, res.Stdout, "LS_COLORS=", "Stdout")

	// Test that whitelist does let through FOO and FOOBAR with FOO*
	command = &com.Command{
		Binary: "env",
		EnvWhiteList: []string{
			"FOO*",
		},
	}

	err = command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err = command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, 0, res.ExitCode, "ExitCode")
	assertive.Contains(t, res.Stdout, "FOO=BAR", "Stdout")
	assertive.Contains(t, res.Stdout, "FOOBAR=BARBAR", "Stdout")
	assertive.DoesNotContain(t, res.Stdout, "LS_COLORS=", "Stdout")
}

func TestEnvBlacklistWhiteList(t *testing.T) {
	t.Setenv("FOO", "BAR")
	t.Setenv("FOOBAR", "BARBAR")

	// Test that if both are specified, only whitelist is taken into account
	command := &com.Command{
		Binary: "env",
		EnvBlackList: []string{
			"*",
			"FOO*",
		},
		EnvWhiteList: []string{
			"*",
		},
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, 0, res.ExitCode, "ExitCode")
	assertive.Contains(t, res.Stdout, "FOO=BAR", "Stdout")
	assertive.Contains(t, res.Stdout, "FOOBAR=BARBAR", "Stdout")
}

func TestEnvAdd(t *testing.T) {
	t.Setenv("FOO", "BAR")
	t.Setenv("BLED", "BLED")
	t.Setenv("BAZ", "OLD")

	command := &com.Command{
		Binary: "env",
		Env: map[string]string{
			"FOO":  "REPLACE",
			"BAR":  "NEW",
			"BLED": "EXPLICIT",
		},
		EnvBlackList: []string{
			"LS_COLORS",
			"BLED",
		},
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	// Confirm explicit Env: declaration overrides os.Environ
	assertive.Contains(t, res.Stdout, "FOO=REPLACE", "Stdout")
	// Confirm explicit Env: declaration does add a new variable
	assertive.Contains(t, res.Stdout, "BAR=NEW", "Stdout")
	// Confirm explicit Env: declaration for unrelated variable does not reset os.Environ
	assertive.Contains(t, res.Stdout, "BAZ=OLD", "Stdout")
	// Confirm that blacklist only operates on os.Environ and not on any explicitly added Env: declaration
	assertive.Contains(t, res.Stdout, "BLED=EXPLICIT", "Stdout")
}

func TestStdoutStderr(t *testing.T) {
	t.Parallel()

	command := &com.Command{
		Binary: "bash",
		Args:   []string{"-c", "--", "printf onstdout; >&2 printf onstderr;"},
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	assertive.IsEqual(t, res.Stdout, "onstdout", "Stdout", "Stdout")
	assertive.IsEqual(t, res.Stderr, "onstderr", "Stderr", "Stderr")
}

func TestTimeoutPlain(t *testing.T) {
	t.Parallel()

	command := &com.Command{
		Binary: "bash",
		// XXX unclear if windows is really able to terminate sleep 5, so, split it up to give it a
		// chance...
		Args:    []string{"-c", "--", "printf one; sleep 1; sleep 1; sleep 1; sleep 1; printf two"},
		Timeout: 1 * time.Second,
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))
	assertive.ErrorIsNil(t, err, "Err")

	start := time.Now()
	res, err := command.Wait()
	end := time.Now()

	assertive.ErrorIs(t, err, com.ErrTimeout, "Err")
	// FIXME? It seems like on windows exitcode is randomly 1 on timeout
	// This is not a problem, as with a time-out we do not care about exit code, but is raising questions
	// about golang underlying implementation / command cancellation mechanism.
	// assertive.IsEqual(t, res.ExitCode, -1, "ExitCode")
	assertive.IsEqual(t, res.Stdout, "one", "Stdout")
	assertive.IsEqual(t, res.Stderr, "", "Stderr")
	assertive.IsLessThan(t, end.Sub(start), 2*time.Second, "Total execution time")
}

func TestTimeoutDelayed(t *testing.T) {
	t.Parallel()

	command := &com.Command{
		Binary: "bash",
		// XXX unclear if windows is really able to terminate sleep 5, so, split it up to give it a
		// chance...
		Args:    []string{"-c", "--", "printf one; sleep 1; sleep 1; sleep 1; sleep 1; printf two"},
		Timeout: 1 * time.Second,
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))
	assertive.ErrorIsNil(t, err, "Err")

	start := time.Now()

	time.Sleep(2 * time.Second)

	res, err := command.Wait()
	end := time.Now()

	assertive.ErrorIs(t, err, com.ErrTimeout, "Err")
	// FIXME? It seems like on windows exitcode is randomly 1 on timeout
	// This is not a problem, as with a time-out we do not care about exit code, but is raising questions
	// about golang underlying implementation / command cancellation mechanism.
	// assertive.IsEqual(t, res.ExitCode, -1, "ExitCode")
	assertive.IsEqual(t, res.Stdout, "one", "Stdout")
	assertive.IsEqual(t, res.Stderr, "", "Stderr")
	assertive.IsLessThan(t, end.Sub(start), 3*time.Second, "Total execution time")
}

func TestPTYStdout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("PTY are not supported on Windows")
	}

	command := &com.Command{
		Binary: "bash",
		Args: []string{
			"-c",
			"--",
			"[ -t 1 ] || { echo not a pty; exit 41; }; printf onstdout; >&2 printf onstderr;",
		},
		Timeout: 1 * time.Second,
	}

	command.WithPTY(false, true, false)

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	assertive.IsEqual(t, res.Stdout, "onstdout", "Stdout")
	assertive.IsEqual(t, res.Stderr, "onstderr", "Stderr")
}

func TestPTYStderr(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("PTY are not supported on Windows")
	}

	command := &com.Command{
		Binary: "bash",
		Args: []string{
			"-c",
			"--",
			"[ -t 2 ] || { echo not a pty; exit 41; }; printf onstdout; >&2 printf onstderr;",
		},
		Timeout: 1 * time.Second,
	}

	command.WithPTY(false, false, true)

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	assertive.IsEqual(t, res.Stdout, "onstdout", "Stdout")
	assertive.IsEqual(t, res.Stderr, "onstderr", "Stderr")
}

func TestPTYBoth(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("PTY are not supported on Windows")
	}

	command := &com.Command{
		Binary: "bash",
		Args: []string{
			"-c", "--", "[ -t 1 ] && [ -t 2 ] || { echo not a pty; exit 41; }; printf onstdout; >&2 printf onstderr;",
		},
		Timeout: 1 * time.Second,
	}

	command.WithPTY(true, true, true)

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, res.ExitCode, 0, "ExitCode")
	assertive.IsEqual(t, res.Stdout, "onstdoutonstderr", "Stdout")
	assertive.IsEqual(t, res.Stderr, "", "Stderr")
}

func TestWriteStdin(t *testing.T) {
	t.Parallel()

	command := &com.Command{
		Binary: "bash",
		Args: []string{
			"-c", "--",
			"read line1; read line2; read line3; printf 'from stdin%s%s%s' \"$line1\" \"$line2\" \"$line3\";",
		},
		Timeout: 1 * time.Second,
	}

	command.WithFeeder(func() io.Reader {
		time.Sleep(100 * time.Millisecond)

		return strings.NewReader("hello first\n")
	})

	command.Feed(strings.NewReader("hello world\n"))
	command.Feed(strings.NewReader("hello again\n"))

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIsNil(t, err, "Err")
	assertive.IsEqual(t, 0, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "from stdinhello firsthello worldhello again", res.Stdout, "Stdout")
}

func TestWritePTYStdin(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("PTY are not supported on Windows")
	}

	command := &com.Command{
		Binary:  "bash",
		Args:    []string{"-c", "--", "[ -t 0 ] || { echo not a pty; exit 41; }; cat /dev/stdin"},
		Timeout: 1 * time.Second,
	}

	command.WithPTY(true, false, false)

	command.WithFeeder(func() io.Reader {
		time.Sleep(100 * time.Millisecond)

		return strings.NewReader("hello first")
	})

	command.Feed(strings.NewReader("hello world"))
	command.Feed(strings.NewReader("hello again"))

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIs(t, err, com.ErrTimeout, "Err")
	assertive.IsEqual(t, -1, res.ExitCode, "ExitCode")
	assertive.IsEqual(t, "hello firsthello worldhello again", res.Stdout, "Stdout")
}

func TestSignalOnCompleted(t *testing.T) {
	t.Parallel()

	var usig os.Signal = syscall.SIGTERM

	command := &com.Command{
		Binary:  "true",
		Timeout: 3 * time.Second,
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	_, err = command.Wait()

	assertive.ErrorIsNil(t, err, "Err")

	err = command.Signal(usig)

	assertive.ErrorIs(t, err, com.ErrFailedSendingSignal, "Err")
}

// FIXME: this is not working as expected, and proc.Signal returns nil error while it should not.
// func TestSignalTooLate(t *testing.T) {
//	t.Parallel()
//
//	var usig os.Signal
//	usig = syscall.SIGTERM
//
//	command := &com.Command{
//		Binary:  "true",
//		Timeout: 3 * time.Second,
//	}
//
//  err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))
//
//	assertive.ErrorIsNil(t, err, "Err")
//
//	time.Sleep(1 * time.Second)
//
//	err = command.Signal(usig)
//
//	assertive.ErrorIs(t, err, com.ErrFailedSendingSignal)
// }

func TestSignalNormal(t *testing.T) {
	t.Parallel()

	var usig os.Signal = syscall.SIGTERM

	sig, ok := usig.(syscall.Signal)
	if !ok {
		panic("sig cast failed")
	}

	command := &com.Command{
		Binary: "bash",
		Args: []string{
			"-c", "--",
			fmt.Sprintf(
				"printf entry; sig_msg () { printf \"caught\"; exit 42; }; trap sig_msg %s; "+
					"printf set; while true; do sleep 0.1; done",
				strconv.Itoa(int(sig)),
			),
		},
		Timeout: 3 * time.Second,
	}

	err := command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	// A bit arbitrary - just want to wait for stdout to go through before sending the signal
	time.Sleep(100 * time.Millisecond)

	_ = command.Signal(usig)

	assertive.ErrorIsNil(t, err, "Err")

	res, err := command.Wait()

	assertive.ErrorIs(t, err, com.ErrExecutionFailed, "Err")
	assertive.IsEqual(t, res.Stdout, "entrysetcaught", "Stdout")
	assertive.IsEqual(t, res.Stderr, "", "Stderr")
	assertive.IsEqual(t, res.ExitCode, 42, "ExitCode")
	assertive.True(t, res.Signal == nil, "Signal")

	command = &com.Command{
		Binary:  "sleep",
		Args:    []string{"10"},
		Timeout: 3 * time.Second,
	}

	err = command.Run(context.WithValue(context.Background(), com.LoggerKey, t))

	assertive.ErrorIsNil(t, err, "Err")

	err = command.Signal(usig)

	assertive.ErrorIsNil(t, err, "Err")

	res, err = command.Wait()

	assertive.ErrorIs(t, err, com.ErrSignaled, "Err")
	assertive.IsEqual(t, res.Stdout, "", "Stdout")
	assertive.IsEqual(t, res.Stderr, "", "Stderr")
	assertive.IsEqual(t, res.Signal, usig, "Signal")
	assertive.IsEqual(t, res.ExitCode, -1, "ExitCode")
}
