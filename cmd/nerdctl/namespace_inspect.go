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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/namespace"
)

func newNamespaceInspectCommand() *cobra.Command {
	namespaceInspectCommand := &cobra.Command{
		Use:           "inspect NAMESPACE",
		Short:         "Display detailed information on one or more namespaces.",
		RunE:          labelInspectAction,
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	namespaceInspectCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	namespaceInspectCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return namespaceInspectCommand
}

func processNamespaceInspectOptions(cmd *cobra.Command) (types.NamespaceInspectOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.NamespaceInspectOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.NamespaceInspectOptions{}, err
	}
	return types.NamespaceInspectOptions{
		GOptions: globalOptions,
		Format:   format,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func labelInspectAction(cmd *cobra.Command, args []string) error {
	options, err := processNamespaceInspectOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return namespace.Inspect(ctx, client, args, options)
}
