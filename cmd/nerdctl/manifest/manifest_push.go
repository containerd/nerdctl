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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/manifest"
)

func PushCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "push [OPTIONS] INDEX/MANIFESTLIST",
		Short:             "Push a manifest list to a registry",
		Args:              cobra.ExactArgs(1),
		RunE:              pushAction,
		ValidArgsFunction: pushShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().Bool("insecure", false, "Allow communication with an insecure registry")
	cmd.Flags().Bool("purge", false, "Remove the manifest list after pushing")
	return cmd
}

func processPushFlags(cmd *cobra.Command) (types.ManifestPushOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ManifestPushOptions{}, err
	}

	insecure, err := cmd.Flags().GetBool("insecure")
	if err != nil {
		return types.ManifestPushOptions{}, err
	}
	purge, err := cmd.Flags().GetBool("purge")
	if err != nil {
		return types.ManifestPushOptions{}, err
	}

	return types.ManifestPushOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		Insecure: insecure,
		Purge:    purge,
	}, nil
}

func pushAction(cmd *cobra.Command, args []string) error {
	pushOptions, err := processPushFlags(cmd)
	if err != nil {
		return err
	}
	err = manifest.Push(cmd.Context(), args[0], pushOptions)
	if err != nil {
		return err
	}
	return nil
}

func pushShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
