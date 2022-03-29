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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/containerd/console"
	"github.com/containerd/typeurl"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newStartCommand() *cobra.Command {
	var startCommand = &cobra.Command{
		Use:               "start [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Start one or more running containers",
		RunE:              startAction,
		ValidArgsFunction: startShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return startCommand
}

func startAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if err := startContainer(ctx, found.Container); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Req)
			return err
		},
	}
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("no such container %s", req)
		}
	}
	return nil
}

func startContainer(ctx context.Context, container containerd.Container) error {
	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	taskCIO := cio.NullIO
	if logURIStr := lab[labels.LogURI]; logURIStr != "" {
		logURI, err := url.Parse(logURIStr)
		if err != nil {
			return err
		}
		taskCIO = cio.LogURI(logURI)
	}
	cStatus := formatter.ContainerStatus(ctx, container)
	if cStatus == "Up" {
		logrus.Warnf("container %s is already running", container.ID())
		return nil
	}
	if oldTask, err := container.Task(ctx, nil); err == nil {
		if _, err := oldTask.Delete(ctx); err != nil {
			logrus.WithError(err).Debug("failed to delete old task")
		}
	}
	flagT, err := getFlagT(ctx, container)
	if err != nil {
		return err
	}
	if flagT {
		var con console.Console
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return fmt.Errorf("failed to SetRaw,err is: %w", err)
		}
		if con == nil {
			return errors.New("got nil con with flagT=true")
		}
		taskCIO = cio.NewCreator(cio.WithStreams(con, con, nil), cio.WithTerminal)
	}
	task, err := container.NewTask(ctx, taskCIO)
	if err != nil {
		return err
	}
	return task.Start(ctx)
}

func getFlagT(ctx context.Context, container containerd.Container) (bool, error) {
	info, err := container.Info(ctx)
	if err != nil {
		return false, err
	}
	spec, err := typeurl.UnmarshalAny(info.Spec)
	if err != nil {
		return false, fmt.Errorf("failed to UnmarshalAny %s,err is: %w", info.Spec, err)
	}
	specInfo, err := json.Marshal(spec)
	if err != nil {
		return false, fmt.Errorf("failed to Marshal %s,err is: %w", spec, err)
	}
	var s specs.Spec
	err = json.Unmarshal(specInfo, &s)
	if err != nil {
		return false, fmt.Errorf("failed to Unmarshal %s,err is: %w", string(specInfo), err)
	}
	return s.Process.Terminal, nil
}

func startShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Running && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
