//go:build unix

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
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"

	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/fifo"
)

type pipes struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

func (p *pipes) closers() []io.Closer {
	return []io.Closer{p.Stdin, p.Stdout, p.Stderr}
}

// copyIO is from https://github.com/containerd/containerd/blob/148d21b1ae0718b75718a09ecb307bb874270f59/cio/io_unix.go#L55
func copyIO(cmd *exec.Cmd, fifos *cio.FIFOSet, ioset *cio.Streams) (*ncio, error) {
	var ctx, cancel = context.WithCancel(context.Background())
	pipes, err := openFifos(ctx, fifos)
	if err != nil {
		cancel()
		return nil, err
	}

	if fifos.Stdin != "" {
		go func() {
			p := bufPool.Get().(*[]byte)
			defer bufPool.Put(p)

			io.CopyBuffer(pipes.Stdin, ioset.Stdin, *p)
			pipes.Stdin.Close()
		}()
	}

	var wg = &sync.WaitGroup{}
	if fifos.Stdout != "" {
		wg.Add(1)
		go func() {
			p := bufPool.Get().(*[]byte)
			defer bufPool.Put(p)

			io.CopyBuffer(ioset.Stdout, pipes.Stdout, *p)
			pipes.Stdout.Close()
			wg.Done()
		}()
	}

	if !fifos.Terminal && fifos.Stderr != "" {
		wg.Add(1)
		go func() {
			p := bufPool.Get().(*[]byte)
			defer bufPool.Put(p)

			io.CopyBuffer(ioset.Stderr, pipes.Stderr, *p)
			pipes.Stderr.Close()
			wg.Done()
		}()
	}

	return &ncio{
		cmd:     cmd,
		config:  fifos.Config,
		wg:      wg,
		closers: append(pipes.closers(), fifos),
		cancel: func() {
			cancel()
			for _, c := range pipes.closers() {
				if c != nil {
					c.Close()
				}
			}
		},
	}, nil
}

func openFifos(ctx context.Context, fifos *cio.FIFOSet) (f pipes, retErr error) {
	defer func() {
		if retErr != nil {
			fifos.Close()
		}
	}()

	if fifos.Stdin != "" {
		if f.Stdin, retErr = fifo.OpenFifo(ctx, fifos.Stdin, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0700); retErr != nil {
			return f, fmt.Errorf("failed to open stdin fifo: %w", retErr)
		}
		defer func() {
			if retErr != nil && f.Stdin != nil {
				f.Stdin.Close()
			}
		}()
	}
	if fifos.Stdout != "" {
		if f.Stdout, retErr = fifo.OpenFifo(ctx, fifos.Stdout, syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0700); retErr != nil {
			return f, fmt.Errorf("failed to open stdout fifo: %w", retErr)
		}
		defer func() {
			if retErr != nil && f.Stdout != nil {
				f.Stdout.Close()
			}
		}()
	}
	if !fifos.Terminal && fifos.Stderr != "" {
		if f.Stderr, retErr = fifo.OpenFifo(ctx, fifos.Stderr, syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0700); retErr != nil {
			return f, fmt.Errorf("failed to open stderr fifo: %w", retErr)
		}
	}
	return f, nil
}
