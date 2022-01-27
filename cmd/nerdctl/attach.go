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
	"fmt"
	"io"
	"os"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newAttachCommand() *cobra.Command {
	var attachCommand = &cobra.Command{
		Use:               "attach [OPTIONS] CONTAINER",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Attach local standard input, output, and error streams to a running container,temporary just support attach the container when use ctr or crictl run a container",
		RunE:              attachAction,
		ValidArgsFunction: attachShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	attachCommand.Flags().SetInterspersed(false)
	attachCommand.Flags().BoolP("stdin", "", false, "Do not attach STDIN")

	return attachCommand
}

func attachAction(cmd *cobra.Command, args []string) error {
	newArg := []string{}
	if len(args) >= 2 && args[1] == "--" {
		newArg = append(newArg, args[:1]...)
		newArg = append(newArg, args[2:]...)
		args = newArg
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("ambiguous ID %q", found.Req)
			}
			return attachActionWithContainer(ctx, cmd, found.Container, client)
		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

func attachActionWithContainer(ctx context.Context, cmd *cobra.Command, container containerd.Container, client *containerd.Client) error {
	flagStdin, err := cmd.Flags().GetBool("stdin")
	if err != nil {
		return err
	}
	spec, err := container.Spec(ctx)
	if err != nil {
		return err
	}

	var (
		con console.Console
		tty = spec.Process.Terminal
	)

	if tty {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	var (
		in     io.Reader
		stdinC = &taskutil.StdinCloser{
			Stdin: os.Stdin,
		}
	)
	if flagStdin {
		in = stdinC
	}
	opt := cio.WithStreams(in, os.Stdout, os.Stderr)
	task, err := container.Task(ctx, cio.NewAttach(opt))
	if err != nil {
		return err
	}
	defer task.Delete(ctx)

	statusC, err := task.Wait(ctx)
	if err != nil {
		return err
	}
	if tty {
		if err := HandleConsoleResize(ctx, task, con); err != nil {
			logrus.WithError(err).Error("console resize")
		}
	} else {
		sigc := commands.ForwardAllSignals(ctx, task)
		defer commands.StopCatch(sigc)
	}

	ec := <-statusC
	code, _, err := ec.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exited error code: %d", int(code))
	}
	return nil
}

func attachShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		statusFilterFn := func(st containerd.ProcessStatus) bool {
			return st == containerd.Running
		}
		return shellCompleteContainerNames(cmd, statusFilterFn)
	} else {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
