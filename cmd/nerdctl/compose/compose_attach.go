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

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
)

func attachCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "attach [flags] SERVICE",
		Short:         "Attach local standard input, output, and error streams to a service's running container",
		Args:          cobra.ExactArgs(1),
		RunE:          attachAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().Int("index", 1, "index of the container if the service has multiple instances.")
	cmd.Flags().String("detach-keys", consoleutil.DefaultDetachKeys, "Override the default detach keys")
	cmd.Flags().Bool("no-stdin", false, "Do not attach STDIN")
	return cmd
}

func attachAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
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
	detachKeys, err := cmd.Flags().GetString("detach-keys")
	if err != nil {
		return err
	}
	noStdin, err := cmd.Flags().GetBool("no-stdin")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	ao := composer.AttachOptions{
		ServiceName: args[0],
		Index:       index,
		DetachKeys:  detachKeys,
		NoStdin:     noStdin,
	}
	return c.Attach(ctx, ao)
}
