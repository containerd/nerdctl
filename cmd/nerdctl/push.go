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

const (
	allowNonDistFlag = "allow-nondistributable-artifacts"
)

func newPushCommand() *cobra.Command {
	var pushCommand = &cobra.Command{
		Use:               "push [flags] NAME[:TAG]",
		Short:             "Push an image or a repository to a registry. Optionally specify \"ipfs://\" or \"ipns://\" scheme to push image to IPFS.",
		Args:              IsExactArgs(1),
		RunE:              pushAction,
		ValidArgsFunction: pushShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	pushCommand.Flags().StringSlice("platform", []string{}, "Push content for a specific platform")
	pushCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	pushCommand.Flags().Bool("all-platforms", false, "Push content for all platforms")
	// #endregion

	pushCommand.Flags().Bool("estargz", false, "Convert the image into eStargz")
	pushCommand.Flags().Bool("ipfs-ensure-image", true, "Ensure the entire contents of the image is locally available before push")
	pushCommand.Flags().String("ipfs-address", "", "multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)")

	// #region sign flags
	pushCommand.Flags().String("sign", "none", "Sign the image (none|cosign")
	pushCommand.RegisterFlagCompletionFunc("sign", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign"}, cobra.ShellCompDirectiveNoFileComp
	})
	pushCommand.Flags().String("cosign-key", "", "Path to the private key file, KMS URI or Kubernetes Secret for --sign=cosign")
	// #endregion

	pushCommand.Flags().Bool(allowNonDistFlag, false, "Allow pushing images with non-distributable blobs")

	return pushCommand
}

func processImagePushCommandOptions(cmd *cobra.Command) (types.ImagePushCommandOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	estargz, err := cmd.Flags().GetBool("estargz")
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	ipfsEnsureImage, err := cmd.Flags().GetBool("ipfs-ensure-image")
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	ipfsAddress, err := cmd.Flags().GetString("ipfs-address")
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	sign, err := cmd.Flags().GetString("sign")
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	cosignKey, err := cmd.Flags().GetString("cosign-key")
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	allowNonDist, err := cmd.Flags().GetBool(allowNonDistFlag)
	if err != nil {
		return types.ImagePushCommandOptions{}, err
	}
	return types.ImagePushCommandOptions{
		GOptions:                       globalOptions,
		Platforms:                      platform,
		AllPlatforms:                   allPlatforms,
		Estargz:                        estargz,
		IpfsEnsureImage:                ipfsEnsureImage,
		IpfsAddress:                    ipfsAddress,
		Sign:                           sign,
		CosignKey:                      cosignKey,
		AllowNondistributableArtifacts: allowNonDist,
	}, nil
}

func pushAction(cmd *cobra.Command, args []string) error {
	options, err := processImagePushCommandOptions(cmd)
	if err != nil {
		return err
	}
	rawRef := args[0]
	return image.Push(cmd.Context(), rawRef, options, cmd.OutOrStdout())
}

func pushShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}
