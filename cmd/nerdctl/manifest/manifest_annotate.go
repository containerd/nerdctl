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

func annotateCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "annotate INDEX/MANIFESTLIST MANIFEST",
		Short:             "Add additional information to a local image manifest",
		Args:              cobra.ExactArgs(2),
		RunE:              annotateAction,
		ValidArgsFunction: annotateShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().String("os", "", "Set operating system")
	cmd.Flags().String("arch", "", "Set architecture")
	cmd.Flags().String("os-version", "", "Set operating system version")
	cmd.Flags().String("variant", "", "Set operating system feature")
	cmd.Flags().StringArray("os-features", []string{}, "Set architecture variant")
	return cmd
}

func processAnnotateFlags(cmd *cobra.Command) (types.ManifestAnnotateOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ManifestAnnotateOptions{}, err
	}

	os, err := cmd.Flags().GetString("os")
	if err != nil {
		return types.ManifestAnnotateOptions{}, err
	}
	arch, err := cmd.Flags().GetString("arch")
	if err != nil {
		return types.ManifestAnnotateOptions{}, err
	}
	osVersion, err := cmd.Flags().GetString("os-version")
	if err != nil {
		return types.ManifestAnnotateOptions{}, err
	}
	variant, err := cmd.Flags().GetString("variant")
	if err != nil {
		return types.ManifestAnnotateOptions{}, err
	}
	osFeatures, err := cmd.Flags().GetStringArray("os-features")
	if err != nil {
		return types.ManifestAnnotateOptions{}, err
	}

	return types.ManifestAnnotateOptions{
		Stdout:     cmd.OutOrStdout(),
		GOptions:   globalOptions,
		Os:         os,
		Arch:       arch,
		OsVersion:  osVersion,
		Variant:    variant,
		OsFeatures: osFeatures,
	}, nil
}

func annotateAction(cmd *cobra.Command, args []string) error {
	annotateOptions, err := processAnnotateFlags(cmd)
	if err != nil {
		return err
	}

	listRef := args[0]
	manifestRef := args[1]

	return manifest.Annotate(cmd.Context(), listRef, manifestRef, annotateOptions)
}

func annotateShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
