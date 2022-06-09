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
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/spf13/cobra"
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

func labelInspectAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	result := make([]interface{}, len(args))
	for index, ns := range args {
		ctx = namespaces.WithNamespace(ctx, ns)
		labels, err := client.NamespaceService().Labels(ctx, ns)
		if err != nil {
			return err
		}
		nsInspect := native.Namespace{
			Name:   ns,
			Labels: &labels,
		}
		result[index] = nsInspect
	}
	return formatSlice(cmd, result)
}
