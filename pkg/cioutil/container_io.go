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

package cioutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/containerd/v2/cmd/containerd-shim-runc-v2/process"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/log"
)

const binaryIOProcTermTimeout = 12 * time.Second // Give logger process 10 seconds for cleanup

// ncio is a basic container IO implementation.
type ncio struct {
	cmd     *exec.Cmd
	config  cio.Config
	wg      *sync.WaitGroup
	closers []io.Closer
	cancel  context.CancelFunc
}

var bufPool = sync.Pool{
	New: func() interface{} {
		buffer := make([]byte, 32<<10)
		return &buffer
	},
}

// closeOnce wraps f's Close so that extra calls return the first result
// instead of "file already closed". The logger pipe ends below are closed
// individually on the success path, but also sit in the error-path closers
// list of NewContainerIO.
func closeOnce(f *os.File) func() error {
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() { err = f.Close() })
		return err
	}
}

// bestEffortWriter forwards writes to w until one fails, then silently
// discards all further writes. The pipe feeding the logging binary is wrapped
// in this before it joins the stdio tee of a foreground container: logging is
// best-effort there, and a dead logging binary (EPIPE once our read ends are
// closed, see NewContainerIO) must not error the whole tee — that would stop
// the copier that drains the container's stdio FIFO and deadlock the
// container. The attach keeps streaming; the log is what goes incomplete.
// https://github.com/containerd/nerdctl/issues/5137
type bestEffortWriter struct {
	w    io.Writer
	dead bool
}

func (b *bestEffortWriter) Write(p []byte) (int, error) {
	// Only ever called from the single stdio copy goroutine of its stream, so
	// no locking is needed.
	if !b.dead {
		if _, err := b.w.Write(p); err != nil {
			b.dead = true
			log.L.WithError(err).Warn("writing container output to the logging binary failed; further output will not be logged")
		}
	}
	return len(p), nil
}

func (c *ncio) Config() cio.Config {
	return c.config
}

func (c *ncio) Wait() {
	if c.wg != nil {
		c.wg.Wait()
	}
}

func (c *ncio) Close() error {

	var lastErr error

	if c.cmd != nil && c.cmd.Process != nil {

		// Send SIGTERM first, so logger process has a chance to flush and exit properly
		if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			lastErr = fmt.Errorf("failed to send SIGTERM: %w", err)

			if err := c.cmd.Process.Kill(); err != nil {
				lastErr = errors.Join(lastErr, fmt.Errorf("failed to kill process after faulty SIGTERM: %w", err))
			}

		}

		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil {
				lastErr = fmt.Errorf("faied to run cmd.wait: %w", err)
			}
		case <-time.After(binaryIOProcTermTimeout):

			err := c.cmd.Process.Kill()
			if err != nil {
				lastErr = fmt.Errorf("failed to kill shim logger process: %w", err)
			}

		}
	}

	for _, closer := range c.closers {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (c *ncio) Cancel() {
	if c.cancel != nil {
		c.cancel()
	}
}

func NewContainerIO(namespace string, logURI string, tty bool, stdin io.Reader, stdout, stderr io.Writer) cio.Creator {
	return func(id string) (_ cio.IO, err error) {
		var (
			cmd     *exec.Cmd
			closers []func() error
			streams = &cio.Streams{
				Terminal: tty,
			}
		)

		defer func() {
			if err == nil {
				return
			}
			result := []error{err}
			for _, fn := range closers {
				result = append(result, fn())
			}
			err = errors.Join(result...)
		}()

		if stdin != nil {
			streams.Stdin = stdin
		}

		var stdoutWriters []io.Writer
		if stdout != nil {
			stdoutWriters = append(stdoutWriters, stdout)
		}

		var stderrWriters []io.Writer
		if stderr != nil {
			stderrWriters = append(stderrWriters, stderr)
		}

		if runtime.GOOS != "windows" && logURI != "" && logURI != "none" {
			// starting logging binary logic is from https://github.com/containerd/containerd/blob/194a1fdd2cde35bc019ef138f30485e27fe0913e/cmd/containerd-shim-runc-v2/process/io.go#L247
			stdoutr, stdoutw, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			closeStdoutR := closeOnce(stdoutr)
			closers = append(closers, closeStdoutR, stdoutw.Close)

			stderrr, stderrw, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			closeStderrR := closeOnce(stderrr)
			closers = append(closers, closeStderrR, stderrw.Close)

			r, w, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			closeR := closeOnce(r)
			closeW := closeOnce(w)
			closers = append(closers, closeR, closeW)

			u, err := url.Parse(logURI)
			if err != nil {
				return nil, err
			}
			cmd = process.NewBinaryCmd(u, id, namespace)
			cmd.ExtraFiles = append(cmd.ExtraFiles, stdoutr, stderrr, w)

			if err := cmd.Start(); err != nil {
				return nil, fmt.Errorf("failed to start binary process with cmdArgs %v (logURI: %s): %w", cmd.Args, logURI, err)
			}

			closers = append(closers, func() error { return cmd.Process.Kill() })

			// close our side of the pipe after start
			if err := closeW(); err != nil {
				return nil, fmt.Errorf("failed to close write pipe after start: %w", err)
			}

			// Close our copies of the stdio read ends that were handed to the
			// logging binary; the child holds its own duplicates via ExtraFiles.
			// This is the equivalent of containerd's binaryIO.CloseAfterStart.
			// If this process kept the read ends open, a logging binary that
			// stops reading (killed, crashed, ...) would never surface as EPIPE
			// on the tee writes below: the stdio copy goroutine would block
			// forever on the full pipe, stop draining the container's stdout
			// FIFO, and deadlock both the container and `nerdctl run` itself
			// (including `nerdctl rm -f` of the wedged container).
			// https://github.com/containerd/nerdctl/issues/5137
			if err := closeStdoutR(); err != nil {
				return nil, fmt.Errorf("failed to close stdout pipe read end after start: %w", err)
			}
			if err := closeStderrR(); err != nil {
				return nil, fmt.Errorf("failed to close stderr pipe read end after start: %w", err)
			}

			// wait for the logging binary to be ready
			// For binary-v2, readiness requires a byte to be written before close.
			// For binary, EOF is treated as ready for backward compatibility.
			b := make([]byte, 1)
			n, err := r.Read(b)
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to read from logging binary: %w", err)
			}
			if u.Scheme == "binary-v2" && n == 0 {
				return nil, errors.New("logging binary did not call ready (it may have crashed or exited prematurely)")
			}
			if err := closeR(); err != nil {
				return nil, fmt.Errorf("failed to close ready pipe read end: %w", err)
			}

			stdoutWriters = append(stdoutWriters, &bestEffortWriter{w: stdoutw})
			stderrWriters = append(stderrWriters, &bestEffortWriter{w: stderrw})
		}

		streams.Stdout = io.MultiWriter(stdoutWriters...)
		streams.Stderr = io.MultiWriter(stderrWriters...)

		if streams.FIFODir == "" {
			streams.FIFODir = defaults.DefaultFIFODir
		}
		fifos, err := cio.NewFIFOSetInDir(streams.FIFODir, id, streams.Terminal)
		if err != nil {
			return nil, err
		}

		if streams.Stdin == nil {
			fifos.Stdin = ""
		}
		if streams.Stdout == nil {
			fifos.Stdout = ""
		}
		if streams.Stderr == nil {
			fifos.Stderr = ""
		}
		return copyIO(cmd, fifos, streams)
	}
}
