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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/namespace"
	"github.com/spf13/cobra"
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

func processNamespaceRemoveCommandOptions(cmd *cobra.Command) (types.NamespaceRemoveCommandOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.NamespaceRemoveCommandOptions{}, err
	}
	cgroup, err := cmd.Flags().GetBool("cgroup")
	if err != nil {
		return types.NamespaceRemoveCommandOptions{}, err
	}
	return types.NamespaceRemoveCommandOptions{
		GOptions: globalOptions,
		CGroup:   cgroup,
	}, nil
}

func namespaceRmAction(cmd *cobra.Command, args []string) error {
	options, err := processNamespaceRemoveCommandOptions(cmd)
	if err != nil {
		return err
	}
	return namespace.Remove(cmd.Context(), args, options, cmd.OutOrStdout())
}
