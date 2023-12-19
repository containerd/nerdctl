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

	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/pkg/process"
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
			return err
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

		if runtime.GOOS != "windows" {
			// starting logging binary logic is from https://github.com/containerd/containerd/blob/194a1fdd2cde35bc019ef138f30485e27fe0913e/cmd/containerd-shim-runc-v2/process/io.go#L247
			stdoutr, stdoutw, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			closers = append(closers, stdoutr.Close, stdoutw.Close)

			stderrr, stderrw, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			closers = append(closers, stderrr.Close, stderrw.Close)

			r, w, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			closers = append(closers, r.Close, w.Close)

			u, err := url.Parse(logURI)
			if err != nil {
				return nil, err
			}
			cmd = process.NewBinaryCmd(u, id, namespace)
			cmd.ExtraFiles = append(cmd.ExtraFiles, stdoutr, stderrr, w)

			if err := cmd.Start(); err != nil {
				return nil, fmt.Errorf("failed to start binary process with cmdArgs %v: %w", cmd.Args, err)
			}

			closers = append(closers, func() error { return cmd.Process.Kill() })

			// close our side of the pipe after start
			if err := w.Close(); err != nil {
				return nil, fmt.Errorf("failed to close write pipe after start: %w", err)
			}

			// wait for the logging binary to be ready
			b := make([]byte, 1)
			if _, err := r.Read(b); err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to read from logging binary: %w", err)
			}

			stdoutWriters = append(stdoutWriters, stdoutw)
			stderrWriters = append(stderrWriters, stderrw)
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
