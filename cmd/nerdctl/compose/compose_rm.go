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
	"fmt"
	"strings"

	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeRemoveCommand() *cobra.Command {
	var composeRemoveCommand = &cobra.Command{
		Use:           "rm [flags] [SERVICE...]",
		Short:         "Remove stopped service containers",
		RunE:          composeRemoveAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeRemoveCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	composeRemoveCommand.Flags().BoolP("stop", "s", false, "Stop containers before removing")
	composeRemoveCommand.Flags().BoolP("volumes", "v", false, "Remove anonymous volumes associated with containers")
	return composeRemoveCommand
}

func composeRemoveAction(cmd *cobra.Command, args []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	if !force {
		var confirm string
		services := "all"
		if len(args) != 0 {
			services = strings.Join(args, ",")
		}
		msg := fmt.Sprintf("This will remove all stopped containers from services: %s.", services)
		msg += "\nAre you sure you want to continue? [y/N] "
		fmt.Fprintf(cmd.OutOrStdout(), "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}

	stop, err := cmd.Flags().GetBool("stop")
	if err != nil {
		return err
	}
	volumes, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return err
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

	rmOpts := composer.RemoveOptions{
		Stop:    stop,
		Volumes: volumes,
	}
	return c.Remove(ctx, rmOpts, args)
}
