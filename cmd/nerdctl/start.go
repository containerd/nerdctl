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
	"runtime"
	"strings"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/errutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil/nettype"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/opencontainers/runtime-spec/specs-go"

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

	startCommand.Flags().SetInterspersed(false)
	startCommand.Flags().BoolP("attach", "a", false, "Attach STDOUT/STDERR and forward signals")

	return startCommand
}

func startAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	flagA, err := cmd.Flags().GetBool("attach")
	if err != nil {
		return err
	}

	if flagA && len(args) > 1 {
		return fmt.Errorf("you cannot start and attach multiple containers at once")
	}

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := startContainer(ctx, found.Container, flagA, client); err != nil {
				return err
			}
			if !flagA {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Req)
				if err != nil {
					return err
				}
			}
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

func startContainer(ctx context.Context, container containerd.Container, flagA bool, client *containerd.Client) error {
	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}

	if err := reconfigNetContainer(ctx, container, client, lab); err != nil {
		return err
	}

	if err := reconfigPIDContainer(ctx, container, client, lab); err != nil {
		return err
	}

	process, err := container.Spec(ctx)
	if err != nil {
		return err
	}
	flagT := process.Process.Terminal
	var con console.Console
	if flagA && flagT {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	logURI := lab[labels.LogURI]

	cStatus := formatter.ContainerStatus(ctx, container)
	if cStatus == "Up" {
		logrus.Warnf("container %s is already running", container.ID())
		return nil
	}
	if err := updateContainerStoppedLabel(ctx, container, false); err != nil {
		return err
	}
	if oldTask, err := container.Task(ctx, nil); err == nil {
		if _, err := oldTask.Delete(ctx); err != nil {
			logrus.WithError(err).Debug("failed to delete old task")
		}
	}
	task, err := taskutil.NewTask(ctx, client, container, flagA, false, flagT, true, con, logURI)
	if err != nil {
		return err
	}

	var statusC <-chan containerd.ExitStatus
	if flagA {
		statusC, err = task.Wait(ctx)
		if err != nil {
			return err
		}
	}

	if err := task.Start(ctx); err != nil {
		return err
	}

	if !flagA {
		return nil
	}
	if flagA && flagT {
		if err := tasks.HandleConsoleResize(ctx, task, con); err != nil {
			logrus.WithError(err).Error("console resize")
		}
	}

	sigc := commands.ForwardAllSignals(ctx, task)
	defer commands.StopCatch(sigc)
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return errutil.NewExitCoderErr(int(code))
	}
	return nil
}

func reconfigNetContainer(ctx context.Context, c containerd.Container, client *containerd.Client, lab map[string]string) error {
	networksJSON, ok := lab[labels.Networks]
	if !ok {
		return nil
	}
	var networks []string
	if err := json.Unmarshal([]byte(networksJSON), &networks); err != nil {
		return err
	}
	netType, err := nettype.Detect(networks)
	if err != nil {
		return err
	}
	if netType == nettype.Container {
		network := strings.Split(networks[0], ":")
		if len(network) != 2 {
			return fmt.Errorf("invalid network: %s, should be \"container:<id|name>\"", networks[0])
		}
		targetCon, err := client.LoadContainer(ctx, network[1])
		if err != nil {
			return err
		}
		netNSPath, err := getContainerNetNSPath(ctx, targetCon)
		if err != nil {
			return err
		}
		spec, err := c.Spec(ctx)
		if err != nil {
			return err
		}
		err = c.Update(ctx, containerd.UpdateContainerOpts(
			containerd.WithSpec(spec, oci.WithLinuxNamespace(
				specs.LinuxNamespace{
					Type: specs.NetworkNamespace,
					Path: netNSPath,
				}))))
		if err != nil {
			return err
		}
	}

	return nil
}

func reconfigPIDContainer(ctx context.Context, c containerd.Container, client *containerd.Client, lab map[string]string) error {
	targetContainerID, ok := lab[labels.PIDContainer]
	if !ok {
		return nil
	}

	if runtime.GOOS != "linux" {
		return errors.New("--pid only supported on linux")
	}

	targetCon, err := client.LoadContainer(ctx, targetContainerID)
	if err != nil {
		return err
	}

	opts, err := generateSharingPIDOpts(ctx, targetCon)
	if err != nil {
		return err
	}

	spec, err := c.Spec(ctx)
	if err != nil {
		return err
	}

	err = c.Update(ctx, containerd.UpdateContainerOpts(
		containerd.WithSpec(spec, oci.Compose(opts...)),
	))
	if err != nil {
		return err
	}

	return nil
}

func startShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Running && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
