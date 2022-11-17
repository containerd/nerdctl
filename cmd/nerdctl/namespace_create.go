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
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/spf13/cobra"
)

func newNamespaceCreateCommand() *cobra.Command {
	namespaceCreateCommand := &cobra.Command{
		Use:           "create NAMESPACE",
		Short:         "Create a new namespace",
		Args:          cobra.MinimumNArgs(1),
		RunE:          namespaceCreateAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	namespaceCreateCommand.Flags().StringArrayP("label", "l", nil, "Set labels for a namespace")
	return namespaceCreateCommand
}

func namespaceCreateAction(cmd *cobra.Command, args []string) error {
	flagVSlice, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	labelsArg := ObjectWithLabelArgs(flagVSlice)
	namespaces := client.NamespaceService()
	return namespaces.Create(ctx, args[0], labelsArg)
}

func ObjectWithLabelArgs(args []string) map[string]string {
	if len(args) >= 1 {
		return commands.LabelArgs(args)
	}
	return nil
}
