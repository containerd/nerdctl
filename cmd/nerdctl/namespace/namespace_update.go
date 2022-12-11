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
	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/spf13/cobra"
)

func NewNamespacelabelUpdateCommand() *cobra.Command {
	namespaceLableCommand := &cobra.Command{
		Use:           "update [flags] NAMESPACE",
		Short:         "Update labels for a namespace",
		RunE:          labelUpdateAction,
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	namespaceLableCommand.Flags().StringArrayP("label", "l", nil, "Set labels for a namespace")
	return namespaceLableCommand
}

func labelUpdateAction(cmd *cobra.Command, args []string) error {
	flagVSlice, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := nerdClient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	labelsArg := ObjectWithLabelArgs(flagVSlice)
	namespaces := client.NamespaceService()
	for k, v := range labelsArg {
		if err := namespaces.SetLabel(ctx, args[0], k, v); err != nil {
			return err
		}
	}
	return nil
}
