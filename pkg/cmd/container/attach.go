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

	"golang.org/x/term"

	"github.com/containerd/console"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/signalutil"
)

// Attach attaches stdin, stdout, and stderr to a running container.
func Attach(ctx context.Context, client *containerd.Client, req string, options types.ContainerAttachOptions) error {
	// Find the container.
	var container containerd.Container
	var cStatus containerd.Status

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

	defer func() {
		containerLabels, err := container.Labels(ctx)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("failed to getting container labels: %s", err)
			return
		}
		rm, err := containerutil.DecodeContainerRmOptLabel(containerLabels[labels.ContainerAutoRemove])
		if err != nil {
			log.G(ctx).WithError(err).Errorf("failed to decode string to bool value: %s", err)
			return
		}
		if rm && cStatus.Status == containerd.Stopped {
			if err = RemoveContainer(ctx, container, options.GOptions, true, true, client); err != nil {
				log.L.WithError(err).Warnf("failed to remove container %s: %s", req, err)
			}
		}
	}()

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
		con, err = consoleutil.Current()
		if err != nil {
			return err
		}
		defer con.Reset()

		if _, err := term.MakeRaw(int(con.Fd())); err != nil {
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
		cStatus, err = task.Status(ctx)
		if err != nil {
			return err
		}
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
