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

package taskutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"sync"
	"syscall"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

// NewTask is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/tasks/tasks_unix.go#L70-L108
func NewTask(ctx context.Context, client *containerd.Client, container containerd.Container, flagI, flagT, flagD bool, con console.Console, logURI string) (containerd.Task, error) {
	var ioCreator cio.Creator
	if flagT {
		if con == nil {
			return nil, errors.New("got nil con with flagT=true")
		}
		var in io.Reader
		if flagI {
			// FIXME: check IsTerminal on Windows too
			if runtime.GOOS != "windows" && !term.IsTerminal(0) {
				return nil, errors.New("the input device is not a TTY")
			}
			in = con
		}
		ioCreator = cio.NewCreator(cio.WithStreams(in, con, nil), cio.WithTerminal)
	} else if flagD && logURI != "" {
		// TODO: support logURI for `nerdctl run -it`
		u, err := url.Parse(logURI)
		if err != nil {
			return nil, err
		}
		ioCreator = cio.LogURI(u)
	} else {
		var in io.Reader
		if flagI {
			plugins, err := client.IntrospectionService().Plugins(ctx, []string{"type==io.containerd.runtime.v2,id==shim"})
			if err != nil {
				return nil, err
			}
			if len(plugins.Plugins) == 0 {
				err = fmt.Errorf("plugin \"io.containerd.runtime.v2.shim\" not found: %w", errdefs.ErrNotFound)
				return nil, fmt.Errorf("containerd v1.6 or later is required for using `nerdctl (run|exec) -i` without `-t`: %w", err)
			}
			stdinC := &StdinCloser{
				Stdin: os.Stdin,
				Closer: func() {
					if t, err := container.Task(ctx, nil); err != nil {
						logrus.WithError(err).Debugf("failed to get task for StdinCloser")
					} else {
						t.CloseIO(ctx, containerd.WithStdinCloser)
					}
				},
			}
			in = stdinC
		}
		ioCreator = cio.NewCreator(cio.WithStreams(in, os.Stdout, os.Stderr))
	}
	t, err := container.NewTask(ctx, ioCreator)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// StdinCloser is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/tasks/exec.go#L181-L194
type StdinCloser struct {
	mu     sync.Mutex
	Stdin  *os.File
	Closer func()
	closed bool
}

func (s *StdinCloser) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, syscall.EBADF
	}
	n, err := s.Stdin.Read(p)
	if err != nil {
		if s.Closer != nil {
			s.Closer()
			s.closed = true
		}
	}
	return n, err
}
