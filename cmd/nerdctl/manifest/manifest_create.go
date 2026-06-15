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

package manifest

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/manifest"
)

func createCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "create INDEX/MANIFESTLIST MANIFEST [MANIFEST...]",
		Short:             "Create a local index/manifest list for annotating and pushing to a registry",
		Args:              cobra.MinimumNArgs(2),
		RunE:              createAction,
		ValidArgsFunction: createShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().Bool("amend", false, "Amend the existing index/manifest list")
	cmd.Flags().Bool("insecure", false, "Allow communication with an insecure registry")
	return cmd
}

func processCreateFlags(cmd *cobra.Command) (types.ManifestCreateOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ManifestCreateOptions{}, err
	}
	amend, err := cmd.Flags().GetBool("amend")
	if err != nil {
		return types.ManifestCreateOptions{}, err
	}
	insecure, err := cmd.Flags().GetBool("insecure")
	if err != nil {
		return types.ManifestCreateOptions{}, err
	}
	return types.ManifestCreateOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		Amend:    amend,
		Insecure: insecure,
	}, nil
}

func createAction(cmd *cobra.Command, args []string) error {
	createOptions, err := processCreateFlags(cmd)
	if err != nil {
		return err
	}

	listRef := args[0]
	manifestRefs := args[1:]

	listRef, err = manifest.Create(cmd.Context(), listRef, manifestRefs, createOptions)
	if err != nil {
		return err
	}

	fmt.Fprintln(createOptions.Stdout, "Created manifest list", listRef)

	return nil
}

func createShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
