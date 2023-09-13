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
	"runtime"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/consoleutil"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/errutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/signalutil"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/sirupsen/logrus"
)

// Run will run a container.
func Run(ctx context.Context, client *containerd.Client, c containerd.Container, createOpt types.ContainerCreateOptions, netManager containerutil.NetworkOptionsManager) error {
	id := c.ID()
	if createOpt.Rm && !createOpt.Detach {
		defer func() {
			// NOTE: OCI hooks (which are used for CNI network setup/teardown on Linux)
			// are not currently supported on Windows, so we must explicitly call
			// network setup/cleanup from the main nerdctl executable.
			if runtime.GOOS == "windows" {
				if err := netManager.CleanupNetworking(ctx, c); err != nil {
					logrus.Warnf("failed to clean up container networking: %s", err)
				}
			}
			if err := RemoveContainer(ctx, c, createOpt.GOptions, true, true); err != nil {
				logrus.WithError(err).Warnf("failed to remove container %s", id)
			}
		}()
	}

	var con console.Console
	if createOpt.TTY && !createOpt.Detach {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	lab, err := c.Labels(ctx)
	if err != nil {
		return err
	}
	logURI := lab[labels.LogURI]
	detachC := make(chan struct{})
	task, err := taskutil.NewTask(ctx, client, c, false, createOpt.Interactive, createOpt.TTY, createOpt.Detach,
		con, logURI, createOpt.DetachKeys, detachC)
	if err != nil {
		return err
	}
	if err := task.Start(ctx); err != nil {
		return err
	}

	if createOpt.Detach {
		fmt.Fprintln(createOpt.Stdout, id)
		return nil
	}
	if createOpt.TTY {
		if err := consoleutil.HandleConsoleResize(ctx, task, con); err != nil {
			logrus.WithError(err).Error("console resize")
		}
	} else {
		sigC := signalutil.ForwardAllSignals(ctx, task)
		defer signalutil.StopCatch(sigC)
	}

	statusC, err := task.Wait(ctx)
	if err != nil {
		return err
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
		if createOpt.Rm {
			if _, taskDeleteErr := task.Delete(ctx); taskDeleteErr != nil {
				logrus.Error(taskDeleteErr)
			}
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
