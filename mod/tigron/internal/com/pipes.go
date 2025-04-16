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
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sync/errgroup"
	"golang.org/x/term"

	"github.com/containerd/nerdctl/mod/tigron/internal/logger"
	"github.com/containerd/nerdctl/mod/tigron/internal/pty"
)

var (
	// ErrFailedCreating could be returned by newStdPipes() on pty creation failure.
	ErrFailedCreating = errors.New("failed acquiring pipe")
	// ErrFailedReading could be returned by the ioGroup in case the go routines fails to read out of a pipe.
	ErrFailedReading = errors.New("failed reading")
	// ErrFailedWriting could be returned by the ioGroup in case the go routines fails to write on a pipe.
	ErrFailedWriting = errors.New("failed writing")
)

type pipe struct {
	reader io.ReadCloser
	writer io.WriteCloser
}

type stdPipes struct {
	ioGroup    *errgroup.Group
	stdin      *pipe
	stdout     *pipe
	stderr     *pipe
	fromStdout string
	fromStderr string
	log        logger.Logger
}

func (pipes *stdPipes) closeCallee() {
	// Failure to close will happen:
	// 1. on a "normal" context timeout:
	// - command is cancelled first (which forcibly closes the callee pipes)
	// - then context deadline is hit
	// - then closeCallee is called here
	// 2. if we have a pty attached on both stdout and stderr
	pipes.log.Helper()
	pipes.log.Log("<- closing callee pipes")

	if pipes.stdin.reader != nil {
		if closeErr := pipes.stdin.reader.Close(); closeErr != nil {
			pipes.log.Log(" x failed closing callee stdin", closeErr)
		}
	}

	if pipes.stdout.writer != nil {
		if closeErr := pipes.stdout.writer.Close(); closeErr != nil {
			pipes.log.Log(" x failed closing callee stdout", closeErr)
		}
	}

	if pipes.stderr.writer != nil {
		if closeErr := pipes.stderr.writer.Close(); closeErr != nil {
			pipes.log.Log(" x failed closing callee stderr", closeErr)
		}
	}
}

func (pipes *stdPipes) closeCaller() {
	pipes.log.Helper()
	pipes.log.Log("<- closing caller pipes")

	if pipes.stdin.writer != nil {
		if closeErr := pipes.stdin.writer.Close(); closeErr != nil {
			pipes.log.Log(" x failed closing caller stdin", closeErr)
		}
	}

	if pipes.stdout.reader != nil {
		if closeErr := pipes.stdout.reader.Close(); closeErr != nil {
			pipes.log.Log(" x failed closing caller stdout", closeErr)
		}
	}

	if pipes.stderr.reader != nil {
		if closeErr := pipes.stderr.reader.Close(); closeErr != nil {
			pipes.log.Log(" x failed closing caller stderr", closeErr)
		}
	}
}

func newStdPipes(
	ctx context.Context,
	log *logger.ConcreteLogger,
	ptyStdout, ptyStderr, ptyStdin bool,
	writers []func() io.Reader,
) (pipes *stdPipes, err error) {
	// Close everything cleanly in case we errored
	defer func() {
		if err != nil {
			pipes.closeCallee()
			pipes.closeCaller()
		}
	}()

	log = log.Set(">", "pipes")
	pipes = &stdPipes{
		stdin:  &pipe{},
		stdout: &pipe{},
		stderr: &pipe{},
		log:    log,
	}

	var (
		mty *os.File
		tty *os.File
	)

	// If we want a pty, configure it now
	if ptyStdout || ptyStderr || ptyStdin {
		pipes.log.Log("<- opening pty")

		mty, tty, err = pty.Open()
		if err != nil {
			pipes.log.Log(" x failed opening pty", err)

			return nil, errors.Join(ErrFailedCreating, err)
		}

		if _, err = term.MakeRaw(int(tty.Fd())); err != nil {
			pipes.log.Log(" x failed making pty raw", err)

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	if ptyStdin {
		pipes.log.Log("<- assigning pty to stdin")

		pipes.stdin.writer = mty
		pipes.stdin.reader = tty
	} else if len(writers) > 0 {
		pipes.log.Log(" * assigning a pipe to stdin as we have writers")

		// Only create a pipe for stdin if we intend on writing to stdin.
		// Otherwise, processes awaiting end of stream will just hang there.
		pipes.stdin.reader, pipes.stdin.writer, err = os.Pipe()
		if err != nil {
			pipes.log.Log(" x failed creating pipe for stdin", err)

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	if ptyStdout {
		pipes.log.Log("<- assigning pty to stdout")

		pipes.stdout.writer = tty
		pipes.stdout.reader = mty
	} else {
		pipes.stdout.reader, pipes.stdout.writer, err = os.Pipe()
		if err != nil {
			pipes.log.Log(" x failed creating pipe for stdout", err)

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	if ptyStderr {
		pipes.log.Log("<- assigning pty to stderr")

		pipes.stderr.writer = tty
		pipes.stderr.reader = mty
	} else {
		pipes.stderr.reader, pipes.stderr.writer, err = os.Pipe()
		if err != nil {
			pipes.log.Log(" x failed creating pipe for stderr", err)

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	// Prepare ioGroup
	pipes.ioGroup, _ = errgroup.WithContext(ctx)

	// Writers to stdin
	pipes.ioGroup.Go(func() error {
		pipes.log.Log("-> about to write to stdin")

		for _, writer := range writers {
			if _, copyErr := io.Copy(pipes.stdin.writer, writer()); copyErr != nil {
				pipes.log.Log(" x failed writing to stdin", copyErr)

				return errors.Join(ErrFailedWriting, copyErr)
			}
		}

		pipes.log.Log("<- done writing to stdin")

		if !ptyStdin && pipes.stdin.writer != nil {
			if closeErr := pipes.stdin.writer.Close(); closeErr != nil {
				pipes.log.Log(" x failed closing caller stdin", closeErr)
			}
		}

		return nil
	})

	// Read stdout...
	pipes.ioGroup.Go(func() error {
		pipes.log.Log("-> about to read stdout")

		buf := &bytes.Buffer{}
		_, copyErr := io.Copy(buf, pipes.stdout.reader)
		pipes.fromStdout = buf.String()

		if copyErr != nil {
			pipes.log.Log(" x failed reading from stdout", copyErr)

			copyErr = errors.Join(ErrFailedReading, copyErr)
		}

		pipes.log.Log("<- done reading stdout")

		return copyErr
	})

	// ... and stderr (if not the same - eg: pty)
	if pipes.stderr.reader != pipes.stdout.reader {
		pipes.ioGroup.Go(func() error {
			pipes.log.Log("-> about to read stderr")

			buf := &bytes.Buffer{}
			_, copyErr := io.Copy(buf, pipes.stderr.reader)
			pipes.fromStderr = buf.String()

			if copyErr != nil {
				pipes.log.Log(" x failed reading from stderr", copyErr)

				copyErr = errors.Join(ErrFailedReading, copyErr)
			}

			pipes.log.Log("<- done reading stderr")

			return copyErr
		})
	}

	return pipes, nil
}
