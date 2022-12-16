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

package compose

import (
	"errors"

	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeExecCommand() *cobra.Command {
	var composeExecCommand = &cobra.Command{
		Use:           "exec [flags] SERVICE COMMAND [ARGS...]",
		Short:         "Execute a command in a running container of the service",
		Args:          cobra.MinimumNArgs(2),
		RunE:          composeExecAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeExecCommand.Flags().SetInterspersed(false)

	composeExecCommand.Flags().BoolP("tty", "t", true, "Allocate a pseudo-TTY")
	composeExecCommand.Flags().BoolP("interactive", "i", true, "Keep STDIN open even if not attached")
	composeExecCommand.Flags().BoolP("detach", "d", false, "Detached mode: Run containers in the background")
	composeExecCommand.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	// env needs to be StringArray, not StringSlice, to prevent "FOO=foo1,foo2" from being split to {"FOO=foo1", "foo2"}
	composeExecCommand.Flags().StringArrayP("env", "e", nil, "Set environment variables")
	// TODO: no-TTY flag
	composeExecCommand.Flags().Bool("privileged", false, "Give extended privileges to the command")
	composeExecCommand.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	composeExecCommand.Flags().Int("index", 1, "index of the container if the service has multiple instances.")

	return composeExecCommand
}

func composeExecAction(cmd *cobra.Command, args []string) error {
	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}
	tty, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return err
	}
	detach, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}
	workdir, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return err
	}
	env, err := cmd.Flags().GetStringArray("env")
	if err != nil {
		return err
	}
	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return err
	}
	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return err
	}
	index, err := cmd.Flags().GetInt("index")
	if err != nil {
		return err
	}

	if index < 1 {
		return errors.New("index starts from 1 and should be equal or greater than 1")
	}
	// https://github.com/containerd/nerdctl/blob/v1.0.0/cmd/nerdctl/exec.go#L116
	if interactive && detach {
		return errors.New("currently flag -i and -d cannot be specified together (FIXME)")
	}
	// https://github.com/containerd/nerdctl/blob/v1.0.0/cmd/nerdctl/exec.go#L122
	if tty && detach {
		return errors.New("currently flag -t and -d cannot be specified together (FIXME)")
	}

	client, ctx, cancel, err := ncclient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}

	eo := composer.ExecOptions{
		ServiceName: args[0],
		Index:       index,

		Interactive: interactive,
		Tty:         tty,
		Detach:      detach,
		WorkDir:     workdir,
		Env:         env,
		Privileged:  privileged,
		User:        user,
		Args:        args[1:],
	}

	return c.Exec(ctx, eo)
}
