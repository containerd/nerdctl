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
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/moby/sys/signal"
	"github.com/sirupsen/logrus"
)

// Kill kills a list of containers
func Kill(ctx context.Context, client *containerd.Client, reqs []string, options types.ContainerKillOptions) error {
	if !strings.HasPrefix(options.KillSignal, "SIG") {
		options.KillSignal = "SIG" + options.KillSignal
	}

	parsedSignal, err := signal.ParseSignal(options.KillSignal)
	if err != nil {
		return err
	}

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := killContainer(ctx, found.Container, parsedSignal); err != nil {
				if errdefs.IsNotFound(err) {
					fmt.Fprintf(options.Stderr, "No such container: %s\n", found.Req)
					os.Exit(1)
				}
				return err
			}
			_, err := fmt.Fprintln(options.Stdout, found.Container.ID())
			return err
		},
	}

	return walker.WalkAll(ctx, reqs, true)
}

func killContainer(ctx context.Context, container containerd.Container, signal syscall.Signal) (err error) {
	defer func() {
		if err != nil {
			containerutil.UpdateErrorLabel(ctx, container, err)
		}
	}()
	if err := containerutil.UpdateExplicitlyStoppedLabel(ctx, container, true); err != nil {
		return err
	}
	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	paused := false

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		return fmt.Errorf("cannot kill container %s: container is not running", container.ID())
	case containerd.Paused, containerd.Pausing:
		paused = true
	default:
	}

	if err := task.Kill(ctx, signal); err != nil {
		return err
	}

	// signal will be sent once resume is finished
	if paused {
		if err := task.Resume(ctx); err != nil {
			logrus.Warnf("cannot unpause container %s: %s", container.ID(), err)
		}
	}
	return nil
}
