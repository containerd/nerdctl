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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/namespace"
)

func newNamespaceRmCommand() *cobra.Command {
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

func processNamespaceRemoveOptions(cmd *cobra.Command) (types.NamespaceRemoveOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.NamespaceRemoveOptions{}, err
	}
	cgroup, err := cmd.Flags().GetBool("cgroup")
	if err != nil {
		return types.NamespaceRemoveOptions{}, err
	}
	return types.NamespaceRemoveOptions{
		GOptions: globalOptions,
		CGroup:   cgroup,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func namespaceRmAction(cmd *cobra.Command, args []string) error {
	options, err := processNamespaceRemoveOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return namespace.Remove(ctx, client, args, options)
}
