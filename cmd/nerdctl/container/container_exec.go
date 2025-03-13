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

	"github.com/spf13/cobra"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func ExecCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "exec [flags] CONTAINER COMMAND [ARG...]",
		Args:              cobra.MinimumNArgs(2),
		Short:             "Run a command in a running container",
		RunE:              execAction,
		ValidArgsFunction: execShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().SetInterspersed(false)

	cmd.Flags().BoolP("tty", "t", false, "Allocate a pseudo-TTY")
	cmd.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")
	cmd.Flags().BoolP("detach", "d", false, "Detached mode: run command in the background")
	cmd.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	// env needs to be StringArray, not StringSlice, to prevent "FOO=foo1,foo2" from being split to {"FOO=foo1", "foo2"}
	cmd.Flags().StringArrayP("env", "e", nil, "Set environment variables")
	// env-file is defined as StringSlice, not StringArray, to allow specifying "--env-file=FILE1,FILE2" (compatible with Podman)
	cmd.Flags().StringSlice("env-file", nil, "Set environment variables from file")
	cmd.Flags().Bool("privileged", false, "Give extended privileges to the command")
	cmd.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	return cmd
}

func execOptions(cmd *cobra.Command) (types.ContainerExecOptions, error) {
	// We do not check if we have a terminal here, as container.Exec calling console.Current will ensure that
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerExecOptions{}, err
	}

	flagI, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}
	flagT, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}
	flagD, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}

	if flagI {
		if flagD {
			return types.ContainerExecOptions{}, errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		if flagD {
			return types.ContainerExecOptions{}, errors.New("currently flag -t and -d cannot be specified together (FIXME)")
		}
	}

	workdir, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}

	envFile, err := cmd.Flags().GetStringSlice("env-file")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}
	env, err := cmd.Flags().GetStringArray("env")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}
	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}
	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return types.ContainerExecOptions{}, err
	}

	return types.ContainerExecOptions{
		GOptions:    globalOptions,
		TTY:         flagT,
		Interactive: flagI,
		Detach:      flagD,
		Workdir:     workdir,
		Env:         env,
		EnvFile:     envFile,
		Privileged:  privileged,
		User:        user,
	}, nil
}

func execAction(cmd *cobra.Command, args []string) error {
	options, err := execOptions(cmd)
	if err != nil {
		return err
	}
	// simulate the behavior of double dash
	newArg := []string{}
	if len(args) >= 2 && args[1] == "--" {
		newArg = append(newArg, args[:1]...)
		newArg = append(newArg, args[2:]...)
		args = newArg
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Exec(ctx, client, args, options)
}

func execShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// show running container names
		statusFilterFn := func(st containerd.ProcessStatus) bool {
			return st == containerd.Running
		}
		return completion.ContainerNames(cmd, statusFilterFn)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
