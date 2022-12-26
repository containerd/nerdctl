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

package fanotify

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/spf13/cobra"
)

type Context struct {
	Enable bool
	Opts   []oci.SpecOpts
}

func GenerateFanotifyOpts(cmd *cobra.Command, flagT bool) (*Context, error) {
	logrus.Warnf("fanotify is unavailable in freebsd platform")

	return &Context{
		Enable: false,
	}, nil
}

func (fanotifyCtx *Context) StartFanotifyMonitor() error {
	if fanotifyCtx == nil {
		return fmt.Errorf("fanotifyCtx is nil")
	}
	return nil
}

func (fanotifyCtx *Context) NewTask(ctx context.Context, client *containerd.Client, container containerd.Container, taskCtx *TaskContext, flagI, flagT, flagD bool) error {
	if flagT && !flagD {
		taskCtx.Console = console.Current()
		defer taskCtx.Console.Reset()
		if err := taskCtx.Console.SetRaw(); err != nil {
			return err
		}
	}

	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	logURI := lab[labels.LogURI]

	taskCtx.Task, err = taskutil.NewTask(ctx, client, container, false, flagI, flagT, flagD, taskCtx.Console, logURI)
	if err != nil {
		return err
	}

	return nil
}

func (fanotifyCtx *Context) PrepareWaiter(ctx context.Context, taskCtx *TaskContext, flagI, flagT bool) error {
	if fanotifyCtx == nil {
		return fmt.Errorf("fanotifyCtx is nil")
	}
	return nil
}

func (fanotifyCtx *Context) StartWaiter(ctx context.Context, container containerd.Container, taskCtx *TaskContext, flagD, rm bool) error {
	if fanotifyCtx == nil {
		return fmt.Errorf("fanotifyCtx is nil")
	}
	if taskCtx == nil {
		return fmt.Errorf("taskCtx is nil")
	}
	var err error
	var code uint32
	var statusC <-chan containerd.ExitStatus
	if !flagD {
		defer func() {
			if rm {
				if _, taskDeleteErr := taskCtx.Task.Delete(ctx); taskDeleteErr != nil {
					logrus.Error(taskDeleteErr)
				}
			}
		}()
		statusC, err = taskCtx.Task.Wait(ctx)
		if err != nil {
			return err
		}
	}

	status := <-statusC
	code, _, err = status.Result()
	if err != nil {
		return err
	}
	taskCtx.ExitCode = code
	return nil
}
