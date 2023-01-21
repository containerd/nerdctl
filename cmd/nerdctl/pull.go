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
	"github.com/containerd/nerdctl/pkg/cmd/image"
	"github.com/spf13/cobra"
)

func newPullCommand() *cobra.Command {
	var pullCommand = &cobra.Command{
		Use:           "pull [flags] NAME[:TAG]",
		Short:         "Pull an image from a registry. Optionally specify \"ipfs://\" or \"ipns://\" scheme to pull image from IPFS.",
		Args:          IsExactArgs(1),
		RunE:          pullAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pullCommand.Flags().String("unpack", "auto", "Unpack the image for the current single platform (auto/true/false)")
	pullCommand.RegisterFlagCompletionFunc("unpack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "true", "false"}, cobra.ShellCompDirectiveNoFileComp
	})

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	pullCommand.Flags().StringSlice("platform", nil, "Pull content for a specific platform")
	pullCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	pullCommand.Flags().Bool("all-platforms", false, "Pull content for all platforms")
	// #endregion

	// #region verify flags
	pullCommand.Flags().String("verify", "none", "Verify the image (none|cosign)")
	pullCommand.RegisterFlagCompletionFunc("verify", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign"}, cobra.ShellCompDirectiveNoFileComp
	})
	pullCommand.Flags().String("cosign-key", "", "Path to the public key file, KMS, URI or Kubernetes Secret for --verify=cosign")
	// #endregion

	pullCommand.Flags().BoolP("quiet", "q", false, "Suppress verbose output")

	pullCommand.Flags().String("ipfs-address", "", "multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)")

	return pullCommand
}

func processPullCommandFlags(cmd *cobra.Command) (types.ImagePullOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return types.ImagePullOptions{}, err
	}

	unpackStr, err := cmd.Flags().GetString("unpack")
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	verifier, err := cmd.Flags().GetString("verify")
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	cosignKey, err := cmd.Flags().GetString("cosign-key")
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	ipfsAddressStr, err := cmd.Flags().GetString("ipfs-address")
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	return types.ImagePullOptions{
		GOptions:     globalOptions,
		AllPlatforms: allPlatforms,
		Platform:     platform,
		Unpack:       unpackStr,
		Quiet:        quiet,
		Verify:       verifier,
		CosignKey:    cosignKey,
		IPFSAddress:  ipfsAddressStr,
		Stdout:       cmd.OutOrStdout(),
		Stderr:       cmd.OutOrStderr(),
	}, nil
}

func pullAction(cmd *cobra.Command, args []string) error {
	var pullOptions, err = processPullCommandFlags(cmd)
	if err != nil {
		return err
	}

	return image.Pull(cmd.Context(), args[0], pullOptions)
}
