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

package container

import (
	"context"
	"errors"
	"fmt"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/signalutil"
)

// Attach attaches stdin, stdout, and stderr to a running container.
func Attach(ctx context.Context, client *containerd.Client, req string, options types.ContainerAttachOptions) error {
	// Find the container.
	var container containerd.Container
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			container = found.Container
			return nil
		},
	}
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return fmt.Errorf("error when trying to find the container: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("no container is found given the string: %s", req)
	} else if n > 1 {
		return fmt.Errorf("more than one containers are found given the string: %s", req)
	}

	// Attach to the container.
	var task containerd.Task
	detachC := make(chan struct{})
	spec, err := container.Spec(ctx)
	if err != nil {
		return fmt.Errorf("failed to get the OCI runtime spec for the container: %w", err)
	}
	var (
		opt cio.Opt
		con console.Console
	)
	if spec.Process.Terminal {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return fmt.Errorf("failed to set the console to raw mode: %w", err)
		}
		closer := func() {
			detachC <- struct{}{}
			// task will be set by container.Task later.
			//
			// We cannot use container.Task(ctx, cio.Load) to get the IO here
			// because the `cancel` field of the returned `*cio` is nil. [1]
			//
			// [1] https://github.com/containerd/containerd/blob/8f756bc8c26465bd93e78d9cd42082b66f276e10/cio/io.go#L358-L359
			io := task.IO()
			if io == nil {
				log.G(ctx).Errorf("got a nil io")
				return
			}
			io.Cancel()
		}
		in, err := consoleutil.NewDetachableStdin(con, options.DetachKeys, closer)
		if err != nil {
			return err
		}
		opt = cio.WithStreams(in, con, nil)
	} else {
		opt = cio.WithStreams(options.Stdin, options.Stdout, options.Stderr)
	}
	task, err = container.Task(ctx, cio.NewAttach(opt))
	if err != nil {
		return fmt.Errorf("failed to attach to the container: %w", err)
	}
	if spec.Process.Terminal {
		if err := consoleutil.HandleConsoleResize(ctx, task, con); err != nil {
			log.G(ctx).WithError(err).Error("console resize")
		}
	}
	sigC := signalutil.ForwardAllSignals(ctx, task)
	defer signalutil.StopCatch(sigC)

	// Wait for the container to exit.
	statusC, err := task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to init an async wait for the container to exit: %w", err)
	}
	select {
	// io.Wait() would return when either 1) the user detaches from the container OR 2) the container is about to exit.
	//
	// If we replace the `select` block with io.Wait() and
	// directly use task.Status() to check the status of the container after io.Wait() returns,
	// it can still be running even though the container is about to exit (somehow especially for Windows).
	//
	// As a result, we need a separate detachC to distinguish from the 2 cases mentioned above.
	case <-detachC:
		io := task.IO()
		if io == nil {
			return errors.New("got a nil IO from the task")
		}
		io.Wait()
	case status := <-statusC:
		code, _, err := status.Result()
		if err != nil {
			return err
		}
		if code != 0 {
			return errutil.NewExitCoderErr(int(code))
		}
	}
	return nil
}
