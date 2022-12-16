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

package namespace

import (
	"fmt"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/spf13/cobra"
)

func NewRmCommand() *cobra.Command {
	namespaceRmCommand := &cobra.Command{
		Use:           "remove [flags] NAMESPACE [NAMESPACE...]",
		Aliases:       []string{"rm"},
		Args:          cobra.MinimumNArgs(1),
		Short:         "Remove one or more namespaces",
		RunE:          namespaceRmAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	namespaceRmCommand.Flags().BoolP("cgroup", "c", false, "delete the namespace's cgroup")
	return namespaceRmCommand
}

func namespaceRmAction(cmd *cobra.Command, args []string) error {
	var exitErr error
	client, ctx, cancel, err := ncclient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	opts, err := namespaceDeleteOpts(cmd)
	if err != nil {
		return err
	}
	namespaces := client.NamespaceService()
	for _, target := range args {
		if err := namespaces.Delete(ctx, target, opts...); err != nil {
			if !errdefs.IsNotFound(err) {
				if exitErr == nil {
					exitErr = fmt.Errorf("unable to delete %s", target)
				}
				log.G(ctx).WithError(err).Errorf("unable to delete %v", target)
				continue
			}
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
		return err
	}
	return exitErr
}
