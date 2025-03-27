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

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"

	"github.com/containerd/nerdctl/mod/tigron/internal/pty"
)

var (
	// ErrFailedCreating could be returned by newStdPipes() on pty creation failure.
	ErrFailedCreating = errors.New("failed acquiring pipe")
	// ErrFailedReading could be returned by the ioGroup in case the go routines fails to read out
	// of a pipe.
	ErrFailedReading = errors.New("failed reading")
	// ErrFailedWriting could be returned by the ioGroup in case the go routines fails to write on a
	// pipe.
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
	log        zerolog.Logger
}

func (pipes *stdPipes) closeCallee() {
	// Failure to close will happen:
	// 1. on a "normal" context timeout:
	// - command is cancelled first (which forcibly closes the callee pipes)
	// - then context deadline is hit
	// - then closeCallee is called here
	// 2. if we have a pty attached on both stdout and stderr
	pipes.log.Trace().Msg("<- closing callee pipes")

	if pipes.stdin.reader != nil {
		cerr := pipes.stdin.reader.Close()
		if cerr != nil {
			pipes.log.Info().Err(cerr).Msg(" x failed closing callee stdin")
		}
	}

	if pipes.stdout.writer != nil {
		cerr := pipes.stdout.writer.Close()
		if cerr != nil {
			pipes.log.Info().Err(cerr).Msg(" x failed closing callee stdout")
		}
	}

	if pipes.stderr.writer != nil {
		cerr := pipes.stderr.writer.Close()
		if cerr != nil {
			pipes.log.Info().Err(cerr).Msg(" x failed closing callee stderr")
		}
	}
}

func (pipes *stdPipes) closeCaller() {
	pipes.log.Trace().Msg("<- closing caller pipes")

	if pipes.stdin.writer != nil {
		cerr := pipes.stdin.writer.Close()
		if cerr != nil {
			pipes.log.Info().Err(cerr).Msg(" x failed closing caller stdin")
		}
	}

	if pipes.stdout.reader != nil {
		cerr := pipes.stdout.reader.Close()
		if cerr != nil {
			pipes.log.Info().Err(cerr).Msg(" x failed closing caller stdout")
		}
	}

	if pipes.stderr.reader != nil {
		cerr := pipes.stderr.reader.Close()
		if cerr != nil {
			pipes.log.Info().Err(cerr).Msg(" x failed closing caller stderr")
		}
	}
}

func newStdPipes(
	ctx context.Context,
	log zerolog.Logger,
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

	pipes = &stdPipes{
		stdin:  &pipe{},
		stdout: &pipe{},
		stderr: &pipe{},
		log:    log.With().Str("module", "pipes").Logger(),
	}

	var (
		mty *os.File
		tty *os.File
	)

	// If we want a pty, configure it now
	if ptyStdout || ptyStderr || ptyStdin {
		pipes.log.Trace().Msg("<- opening pty")

		mty, tty, err = pty.Open()
		if err != nil {
			pipes.log.Error().Err(err).Msg(" x failed opening pty")

			return nil, errors.Join(ErrFailedCreating, err)
		}

		if _, err = term.MakeRaw(int(tty.Fd())); err != nil {
			pipes.log.Error().Err(err).Msg(" x failed making pty raw")

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	if ptyStdin {
		pipes.log.Trace().Msg("<- assigning pty to stdin")

		pipes.stdin.writer = mty
		pipes.stdin.reader = tty
	} else if len(writers) > 0 {
		pipes.log.Trace().Msg(" * assigning a pipe to stdin as we have writers")

		// Only create a pipe for stdin if we intend on writing to stdin.
		// Otherwise, processes awaiting end of stream will just hang there.
		pipes.stdin.reader, pipes.stdin.writer, err = os.Pipe()
		if err != nil {
			pipes.log.Error().Err(err).Msg(" x failed creating pipe for stdin")

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	if ptyStdout {
		pipes.log.Trace().Msg("<- assigning pty to stdout")

		pipes.stdout.writer = tty
		pipes.stdout.reader = mty
	} else {
		pipes.stdout.reader, pipes.stdout.writer, err = os.Pipe()
		if err != nil {
			pipes.log.Error().Err(err).Msg(" x failed creating pipe for stdout")

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	if ptyStderr {
		pipes.log.Trace().Msg("<- assigning pty to stderr")

		pipes.stderr.writer = tty
		pipes.stderr.reader = mty
	} else {
		pipes.stderr.reader, pipes.stderr.writer, err = os.Pipe()
		if err != nil {
			pipes.log.Error().Err(err).Msg(" x failed creating pipe for stderr")

			return nil, errors.Join(ErrFailedCreating, err)
		}
	}

	// Prepare ioGroup
	pipes.ioGroup, _ = errgroup.WithContext(ctx)

	// Writers to stdin
	pipes.ioGroup.Go(func() error {
		pipes.log.Trace().Msg("-> about to write to stdin")

		for _, writer := range writers {
			if _, copyErr := io.Copy(pipes.stdin.writer, writer()); copyErr != nil {
				pipes.log.Error().Err(copyErr).Msg(" x failed writing to stdin")

				return errors.Join(ErrFailedWriting, copyErr)
			}
		}

		pipes.log.Trace().Msg("<- done writing to stdin")

		return nil
	})

	// Read stdout...
	pipes.ioGroup.Go(func() error {
		pipes.log.Trace().Msg("-> about to read stdout")

		buf := &bytes.Buffer{}
		_, copyErr := io.Copy(buf, pipes.stdout.reader)
		pipes.fromStdout = buf.String()

		if copyErr != nil {
			pipes.log.Error().Err(copyErr).Msg(" x failed reading from stdout")

			copyErr = errors.Join(ErrFailedReading, copyErr)
		}

		pipes.log.Trace().Msg("<- done reading stdout")

		return copyErr
	})

	// ... and stderr (if not the same - eg: pty)
	if pipes.stderr.reader != pipes.stdout.reader {
		pipes.ioGroup.Go(func() error {
			pipes.log.Trace().Msg("-> about to read stderr")

			buf := &bytes.Buffer{}
			_, copyErr := io.Copy(buf, pipes.stderr.reader)
			pipes.fromStderr = buf.String()

			if copyErr != nil {
				pipes.log.Error().Err(copyErr).Msg(" x failed reading from stderr")

				copyErr = errors.Join(ErrFailedReading, copyErr)
			}

			pipes.log.Trace().Msg("<- done reading stderr")

			return copyErr
		})
	}

	return pipes, nil
}
