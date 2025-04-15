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

package com

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/internal/logger"
)

const (
	defaultTimeout = 10 * time.Second
	delayAfterWait = 100 * time.Millisecond
)

var (
	// ErrTimeout is returned by Wait() in case a command fail to complete within allocated time.
	ErrTimeout = errors.New("command timed out")
	// ErrFailedStarting is returned by Run() and Wait() in case a command fails to start (eg:
	// binary missing).
	ErrFailedStarting = errors.New("command failed starting")
	// ErrSignaled is returned by Wait() if a signal was sent to the command while running.
	ErrSignaled = errors.New("command execution signaled")
	// ErrExecutionFailed is returned by Wait() when a command executes but returns a non-zero error code.
	ErrExecutionFailed = errors.New("command returned a non-zero exit code")
	// ErrFailedSendingSignal may happen if sending a signal to an already terminated process.
	ErrFailedSendingSignal = errors.New("failed sending signal")

	// ErrExecAlreadyStarted is a system error normally indicating a bogus double call to Run().
	ErrExecAlreadyStarted = errors.New("command has already been started (double `Run`)")
	// ErrExecNotStarted is a system error normally indicating that Wait() has been called without first calling Run().
	ErrExecNotStarted = errors.New("command has not been started (call `Run` first)")
	// ErrExecAlreadyFinished is a system error indicating a double call to Wait().
	ErrExecAlreadyFinished = errors.New("command is already finished")

	errExecutionCancelled = errors.New("command execution cancelled")
)

type contextKey string

// LoggerKey defines the key to attach a logger to on the context.
const LoggerKey = contextKey("logger")

// Result carries the resulting output of a command once it has finished.
type Result struct {
	Environ  []string
	Stdout   string
	Stderr   string
	ExitCode int
	Signal   os.Signal
	Duration time.Duration
}

type execution struct {
	//nolint:containedctx // Is there a way around this?
	context context.Context
	cancel  context.CancelFunc
	command *exec.Cmd
	pipes   *stdPipes
	log     logger.Logger
	err     error
}

// Command is a thin wrapper on-top of golang exec.Command.
type Command struct {
	Binary      string
	PrependArgs []string
	Args        []string
	WrapBinary  string
	WrapArgs    []string
	Timeout     time.Duration

	WorkingDir   string
	Env          map[string]string
	EnvBlackList []string
	EnvWhiteList []string

	writers []func() io.Reader

	ptyStdout bool
	ptyStderr bool
	ptyStdin  bool

	exec      *execution
	mutex     sync.Mutex
	result    *Result
	startTime time.Time
}

// Clone does just duplicate a command, resetting its execution.
func (gc *Command) Clone() *Command {
	com := &Command{
		Binary:      gc.Binary,
		PrependArgs: append([]string(nil), gc.PrependArgs...),
		Args:        append([]string(nil), gc.Args...),
		WrapBinary:  gc.WrapBinary,
		WrapArgs:    append([]string(nil), gc.WrapArgs...),
		Timeout:     gc.Timeout,

		WorkingDir:   gc.WorkingDir,
		Env:          map[string]string{},
		EnvBlackList: append([]string(nil), gc.EnvBlackList...),
		EnvWhiteList: append([]string(nil), gc.EnvWhiteList...),

		writers: append([]func() io.Reader(nil), gc.writers...),

		ptyStdout: gc.ptyStdout,
		ptyStderr: gc.ptyStderr,
		ptyStdin:  gc.ptyStdin,
	}

	for k, v := range gc.Env {
		com.Env[k] = v
	}

	return com
}

// WithPTY requests that the command be executed with a pty for std streams.
// Parameters allow showing which streams are to be tied to the pty.
// This command has no effect if Run has already been called.
func (gc *Command) WithPTY(stdin, stdout, stderr bool) {
	gc.ptyStdout = stdout
	gc.ptyStderr = stderr
	gc.ptyStdin = stdin
}

// WithFeeder ensures that the provider function will be executed and its output fed to the command stdin.
// WithFeeder, like Feed, can be used multiple times, and writes will be performed sequentially, in order.
// This command has no effect if Run has already been called.
func (gc *Command) WithFeeder(writers ...func() io.Reader) {
	gc.writers = append(gc.writers, writers...)
}

// Feed ensures that the provider reader will be copied on the command stdin.
// Feed, like WithFeeder, can be used multiple times, and writes will be performed in sequentially, in order.
// This command has no effect if Run has already been called.
func (gc *Command) Feed(reader io.Reader) {
	gc.writers = append(gc.writers, func() io.Reader {
		return reader
	})
}

// Run starts the command in the background.
// It may error out immediately if the command fails to start (ErrFailedStarting).
func (gc *Command) Run(parentCtx context.Context) error {
	// Lock
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	// Protect against dumb calls
	if gc.result != nil {
		return ErrExecAlreadyFinished
	} else if gc.exec != nil {
		return ErrExecAlreadyStarted
	}

	var (
		ctx       context.Context
		ctxCancel context.CancelFunc
		pipes     *stdPipes
		cmd       *exec.Cmd
		err       error
	)

	// Get a timing-out context
	if gc.Timeout == 0 {
		gc.Timeout = defaultTimeout
	}

	ctx, ctxCancel = context.WithTimeout(parentCtx, gc.Timeout)
	gc.startTime = time.Now()

	// Create a contextual command, set the logger
	cmd = gc.buildCommand(ctx)
	// Get a debug-logger from the context
	var (
		log logger.Logger
		ok  bool
	)

	if log, ok = parentCtx.Value(LoggerKey).(logger.Logger); !ok {
		log = nil
	}

	conLog := logger.NewLogger(log).Set("command", cmd.String())
	// FIXME: this is manual silencing of pipe logs (very noisy)
	// It should be possible to enable this with some debug flag.
	// Note that one probably never want this on unless they are actually debugging pipes issues...
	emLog := logger.NewLogger(nil).Set("command", cmd.String())

	gc.exec = &execution{
		context: ctx,
		cancel:  ctxCancel,
		command: cmd,
		log:     conLog,
	}

	// Prepare pipes
	pipes, err = newStdPipes(ctx, emLog, gc.ptyStdout, gc.ptyStderr, gc.ptyStdin, gc.writers)
	if err != nil {
		ctxCancel()

		gc.exec.err = errors.Join(ErrFailedStarting, err)

		// No wrapping here - we do not even have pipes, and the command has not been started.

		return gc.exec.err
	}

	// Attach pipes
	gc.exec.pipes = pipes
	cmd.Stdout = pipes.stdout.writer
	cmd.Stderr = pipes.stderr.writer
	cmd.Stdin = pipes.stdin.reader

	// Start it
	if err = cmd.Start(); err != nil {
		// On failure, can the context, wrap whatever we have and return
		gc.exec.log.Log("start failed", err)

		gc.exec.err = errors.Join(ErrFailedStarting, err)

		_ = gc.wrap()

		defer ctxCancel()

		return gc.exec.err
	}

	select {
	case <-ctx.Done():
		// There is no good reason for this to happen, so, log it
		err = gc.wrap()

		gc.exec.log.Log("stdout", gc.result.Stdout)
		gc.exec.log.Log("stderr", gc.result.Stderr)
		gc.exec.log.Log("exitcode", gc.result.ExitCode)
		gc.exec.log.Log("err", err)
		gc.exec.log.Log("ctxerr", ctx.Err())

		return err
	default:
	}

	return nil
}

// Wait should be called after Run(), and will return the outcome of the command execution.
func (gc *Command) Wait() (*Result, error) {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	switch {
	case gc.exec == nil:
		return nil, ErrExecNotStarted
	case gc.exec.err != nil:
		return gc.result, gc.exec.err
	case gc.result != nil:
		return gc.result, ErrExecAlreadyFinished
	}

	// Cancel the context in any case now
	defer gc.exec.cancel()

	// Wait for the command
	_ = gc.exec.command.Wait()

	// Capture timeout and cancellation
	select {
	case <-gc.exec.context.Done():
	default:
	}

	// Wrap the results and return
	err := gc.wrap()

	return gc.result, err
}

// Signal sends a signal to the command. It should be called after Run() but before Wait().
func (gc *Command) Signal(sig os.Signal) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if gc.exec == nil {
		return ErrExecNotStarted
	}

	err := gc.exec.command.Process.Signal(sig)
	if err != nil {
		err = errors.Join(ErrFailedSendingSignal, err)
	}

	return err
}

func (gc *Command) wrap() error {
	pipes := gc.exec.pipes
	cmd := gc.exec.command
	ctx := gc.exec.context

	// Close and drain the pipes
	pipes.closeCallee()
	_ = pipes.ioGroup.Wait()
	pipes.closeCaller()

	// Get the status, exitCode, signal, error
	var (
		status   syscall.WaitStatus
		signal   os.Signal
		exitCode int
		err      error
	)

	// XXXgolang: this is troubling. cmd.ProcessState.ExitCode() is always fine, even if cmd.ProcessState is nil.
	exitCode = cmd.ProcessState.ExitCode()

	if cmd.ProcessState != nil {
		var ok bool
		if status, ok = cmd.ProcessState.Sys().(syscall.WaitStatus); !ok {
			panic("failed casting process state sys")
		}

		if status.Signaled() {
			signal = status.Signal()
			err = ErrSignaled
		} else if exitCode != 0 {
			err = ErrExecutionFailed
		}
	}

	// Catch-up on the context.
	switch ctx.Err() {
	case context.DeadlineExceeded:
		err = ErrTimeout
	case context.Canceled:
		err = errExecutionCancelled
	default:
	}

	// Stuff everything in Result and return err.
	gc.result = &Result{
		ExitCode: exitCode,
		Stdout:   pipes.fromStdout,
		Stderr:   pipes.fromStderr,
		Environ:  cmd.Environ(),
		Signal:   signal,
		Duration: time.Since(gc.startTime),
	}

	if gc.exec.err == nil {
		gc.exec.err = err
	}

	return gc.exec.err
}

func (gc *Command) buildCommand(ctx context.Context) *exec.Cmd {
	// Build arguments and binary.
	args := gc.Args
	if gc.PrependArgs != nil {
		args = append(gc.PrependArgs, args...)
	}

	binary := gc.Binary

	if gc.WrapBinary != "" {
		args = append([]string{gc.Binary}, args...)
		args = append(gc.WrapArgs, args...)
		binary = gc.WrapBinary
	}

	//nolint:gosec
	cmd := exec.CommandContext(ctx, binary, args...)

	// Add dir.
	cmd.Dir = gc.WorkingDir

	// Set wait delay after waits returns.
	cmd.WaitDelay = delayAfterWait

	// Build env.
	cmd.Env = []string{}

	const (
		star  = "*"
		equal = "="
	)

	for _, envValue := range os.Environ() {
		add := true

		for _, needle := range gc.EnvBlackList {
			if strings.HasSuffix(needle, star) {
				needle = strings.TrimSuffix(needle, star)
			} else if needle != star && !strings.Contains(needle, equal) {
				needle += equal
			}

			if needle == star || strings.HasPrefix(envValue, needle) {
				add = false

				break
			}
		}

		if len(gc.EnvWhiteList) > 0 {
			add = false

			for _, needle := range gc.EnvWhiteList {
				if strings.HasSuffix(needle, star) {
					needle = strings.TrimSuffix(needle, star)
				} else if needle != star && !strings.Contains(needle, equal) {
					needle += equal
				}

				if needle == star || strings.HasPrefix(envValue, needle) {
					add = true

					break
				}
			}
		}

		if add {
			cmd.Env = append(cmd.Env, envValue)
		}
	}

	// Attach any explicit env we have
	for k, v := range gc.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Attach platform ProcAttr and get optional custom cancellation routine.
	if cancellation := addAttr(cmd); cancellation != nil {
		cmd.Cancel = func() error {
			gc.exec.log.Log("command cancelled")

			// Call the platform dependent cancellation routine.
			return cancellation()
		}
	}

	return cmd
}
