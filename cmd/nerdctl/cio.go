/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package main

import (
	"context"
	"io"
	"net/url"
	"os"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/pkg/errors"
)

// newTask is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/tasks/tasks_unix.go#L70-L108
func newTask(ctx context.Context, client *containerd.Client, container containerd.Container, flagI, flagT, flagD bool, con console.Console, logURI string) (containerd.Task, error) {
	stdinC := &stdinCloser{
		stdin: os.Stdin,
	}
	var ioCreator cio.Creator
	if flagT {
		if con == nil {
			return nil, errors.New("got nil con with flagT=true")
		}
		ioCreator = cio.NewCreator(cio.WithStreams(con, con, nil), cio.WithTerminal)
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
			in = stdinC
		}
		ioCreator = cio.NewCreator(cio.WithStreams(in, os.Stdout, os.Stderr))
	}
	t, err := container.NewTask(ctx, ioCreator)
	if err != nil {
		return nil, err
	}
	stdinC.closer = func() {
		t.CloseIO(ctx, containerd.WithStdinCloser)
	}
	return t, nil
}

// stdinCloser is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/tasks/exec.go#L181-L194
type stdinCloser struct {
	stdin  *os.File
	closer func()
}

func (s *stdinCloser) Read(p []byte) (int, error) {
	n, err := s.stdin.Read(p)
	if err == io.EOF {
		if s.closer != nil {
			s.closer()
		}
	}
	return n, err
}
