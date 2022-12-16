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
	"errors"
	"fmt"
	"runtime"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/action"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/run"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRunCommand() *cobra.Command {
	shortHelp := "Run a command in a new container. Optionally specify \"ipfs://\" or \"ipns://\" scheme to pull image from IPFS."
	longHelp := shortHelp
	switch runtime.GOOS {
	case "windows":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl run` is experimental on Windows and currently broken (https://github.com/containerd/nerdctl/issues/28)"
	case "freebsd":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl run` is experimental on FreeBSD and currently requires `--net=none` (https://github.com/containerd/nerdctl/blob/main/docs/freebsd.md)"
	}
	var runCommand = &cobra.Command{
		Use:               "run [flags] IMAGE [COMMAND] [ARG...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              runAction,
		ValidArgsFunction: completion.RunShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	runCommand.Flags().SetInterspersed(false)
	run.SetCreateFlags(runCommand)

	runCommand.Flags().BoolP("detach", "d", false, "Run container in background and print container ID")

	return runCommand
}

// runAction is heavily based on ctr implementation:
// https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
func runAction(cmd *cobra.Command, args []string) error {
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := ncclient.NewWithPlatform(cmd, platform)
	if err != nil {
		return err
	}
	defer cancel()

	flagD, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}
	flagI, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}
	flagT, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return err
	}
	container, gc, err := run.CreateContainer(ctx, cmd, client, args, platform, flagI, flagT, flagD)
	if err != nil {
		if gc != nil {
			defer gc()
		}
		return err
	}

	id := container.ID()
	rm, err := cmd.Flags().GetBool("rm")
	if err != nil {
		return err
	}
	if rm {
		if flagD {
			return errors.New("flag -d and --rm cannot be specified together")
		}
		defer func() {
			if err := action.RemoveContainer(ctx, cmd, container, true, true); err != nil {
				logrus.WithError(err).Warnf("failed to remove container %s", id)
			}
		}()
	}

	var con console.Console
	if flagT {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	logURI := lab[labels.LogURI]

	task, err := taskutil.NewTask(ctx, client, container, flagI, flagT, flagD, con, logURI)
	if err != nil {
		return err
	}
	var statusC <-chan containerd.ExitStatus
	if !flagD {
		defer func() {
			if rm {
				if _, taskDeleteErr := task.Delete(ctx); taskDeleteErr != nil {
					logrus.Error(taskDeleteErr)
				}
			}
		}()
		statusC, err = task.Wait(ctx)
		if err != nil {
			return err
		}
	}

	if err := task.Start(ctx); err != nil {
		return err
	}

	if flagD {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", id)
		return nil
	}
	if flagT {
		if err := tasks.HandleConsoleResize(ctx, task, con); err != nil {
			logrus.WithError(err).Error("console resize")
		}
	} else {
		sigc := commands.ForwardAllSignals(ctx, task)
		defer commands.StopCatch(sigc)
	}
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return common.ExitCodeError{
			Code: int(code),
		}
	}
	return nil
}
