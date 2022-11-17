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
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newExecCommand() *cobra.Command {
	var execCommand = &cobra.Command{
		Use:               "exec [OPTIONS] CONTAINER COMMAND [ARG...]",
		Args:              cobra.MinimumNArgs(2),
		Short:             "Run a command in a running container",
		RunE:              execAction,
		ValidArgsFunction: execShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	execCommand.Flags().SetInterspersed(false)

	execCommand.Flags().BoolP("tty", "t", false, "(Currently -t needs to correspond to -i)")
	execCommand.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")
	execCommand.Flags().BoolP("detach", "d", false, "Detached mode: run command in the background")
	execCommand.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	// env needs to be StringArray, not StringSlice, to prevent "FOO=foo1,foo2" from being split to {"FOO=foo1", "foo2"}
	execCommand.Flags().StringArrayP("env", "e", nil, "Set environment variables")
	// env-file is defined as StringSlice, not StringArray, to allow specifying "--env-file=FILE1,FILE2" (compatible with Podman)
	execCommand.Flags().StringSlice("env-file", nil, "Set environment variables from file")
	execCommand.Flags().Bool("privileged", false, "Give extended privileges to the command")
	execCommand.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	return execCommand
}

func execAction(cmd *cobra.Command, args []string) error {
	// simulate the behavior of double dash
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
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			return execActionWithContainer(ctx, cmd, args, found.Container, client)
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

func execActionWithContainer(ctx context.Context, cmd *cobra.Command, args []string, container containerd.Container, client *containerd.Client) error {
	flagI, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}
	flagT, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return err
	}
	flagD, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}

	if flagI {
		if flagD {
			return errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		if flagD {
			return errors.New("currently flag -t and -d cannot be specified together (FIXME)")
		}
	}

	pspec, err := generateExecProcessSpec(ctx, cmd, args, container, client)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}
	var (
		ioCreator cio.Creator
		in        io.Reader
		stdinC    = &taskutil.StdinCloser{
			Stdin: os.Stdin,
		}
	)

	if flagI {
		in = stdinC
	}
	cioOpts := []cio.Opt{cio.WithStreams(in, os.Stdout, os.Stderr)}
	if flagT {
		cioOpts = append(cioOpts, cio.WithTerminal)
	}
	ioCreator = cio.NewCreator(cioOpts...)

	execID := "exec-" + idgen.GenerateID()
	process, err := task.Exec(ctx, execID, pspec, ioCreator)
	if err != nil {
		return err
	}
	stdinC.Closer = func() {
		process.CloseIO(ctx, containerd.WithStdinCloser)
	}
	// if detach, we should not call this defer
	if !flagD {
		defer process.Delete(ctx)
	}

	statusC, err := process.Wait(ctx)
	if err != nil {
		return err
	}

	var con console.Console
	if flagT {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}
	if !flagD {
		if flagT {
			if err := tasks.HandleConsoleResize(ctx, process, con); err != nil {
				logrus.WithError(err).Error("console resize")
			}
		} else {
			sigc := commands.ForwardAllSignals(ctx, process)
			defer commands.StopCatch(sigc)
		}
	}

	if err := process.Start(ctx); err != nil {
		return err
	}
	if flagD {
		return nil
	}
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exec failed with exit code %d", code)
	}
	return nil
}

func generateExecProcessSpec(ctx context.Context, cmd *cobra.Command, args []string, container containerd.Container, client *containerd.Client) (*specs.Process, error) {
	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, err
	}
	userOpts, err := generateUserOpts(cmd)
	if err != nil {
		return nil, err
	}
	if userOpts != nil {
		c, err := container.Info(ctx)
		if err != nil {
			return nil, err
		}
		for _, opt := range userOpts {
			if err := opt(ctx, client, &c, spec); err != nil {
				return nil, err
			}
		}
	}

	pspec := spec.Process
	flagT, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return nil, err
	}
	pspec.Terminal = flagT
	pspec.Args = args[1:]

	workdir, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return nil, err
	}
	if workdir != "" {
		pspec.Cwd = workdir
	}
	envFile, err := cmd.Flags().GetStringSlice("env-file")
	if err != nil {
		return nil, err
	}
	if envFiles := strutil.DedupeStrSlice(envFile); len(envFiles) > 0 {
		env, err := parseEnvVars(envFiles)
		if err != nil {
			return nil, err
		}
		pspec.Env = append(pspec.Env, env...)
	}
	env, err := cmd.Flags().GetStringArray("env")
	if err != nil {
		return nil, err
	}
	pspec.Env = append(pspec.Env, strutil.DedupeStrSlice(env)...)

	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return nil, err
	}
	if privileged {
		err = setExecCapabilities(pspec)
		if err != nil {
			return nil, err
		}
	}

	return pspec, nil
}

func execShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// show running container names
		statusFilterFn := func(st containerd.ProcessStatus) bool {
			return st == containerd.Running
		}
		return shellCompleteContainerNames(cmd, statusFilterFn)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
