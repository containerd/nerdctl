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

package image

import (
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
)

const (
	allowNonDistFlag = "allow-nondistributable-artifacts"
)

func PushCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "push [flags] NAME[:TAG]",
		Short:             "Push an image or a repository to a registry. Optionally specify \"ipfs://\" or \"ipns://\" scheme to push image to IPFS.",
		Args:              helpers.IsExactArgs(1),
		RunE:              pushAction,
		ValidArgsFunction: pushShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	cmd.Flags().StringSlice("platform", []string{}, "Push content for a specific platform")
	cmd.RegisterFlagCompletionFunc("platform", completion.Platforms)
	cmd.Flags().Bool("all-platforms", false, "Push content for all platforms")
	// #endregion

	cmd.Flags().Bool("estargz", false, "Convert the image into eStargz")
	cmd.Flags().Bool("ipfs-ensure-image", true, "Ensure the entire contents of the image is locally available before push")
	cmd.Flags().String("ipfs-address", "", "multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)")

	// #region sign flags
	cmd.Flags().String("sign", "none", "Sign the image (none|cosign|notation")
	cmd.RegisterFlagCompletionFunc("sign", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign", "notation"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("cosign-key", "", "Path to the private key file, KMS URI or Kubernetes Secret for --sign=cosign")
	cmd.Flags().String("notation-key-name", "", "Signing key name for a key previously added to notation's key list for --sign=notation")
	// #endregion

	// #region soci flags
	cmd.Flags().Int64("soci-span-size", -1, "Span size that soci index uses to segment layer data. Default is 4 MiB.")
	cmd.Flags().Int64("soci-min-layer-size", -1, "Minimum layer size to build zTOC for. Smaller layers won't have zTOC and not lazy pulled. Default is 10 MiB.")
	// #endregion

	cmd.Flags().BoolP("quiet", "q", false, "Suppress verbose output")

	cmd.Flags().Bool(allowNonDistFlag, false, "Allow pushing images with non-distributable blobs")

	return cmd
}

func pushOptions(cmd *cobra.Command) (types.ImagePushOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	estargz, err := cmd.Flags().GetBool("estargz")
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	ipfsEnsureImage, err := cmd.Flags().GetBool("ipfs-ensure-image")
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	ipfsAddress, err := cmd.Flags().GetString("ipfs-address")
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	allowNonDist, err := cmd.Flags().GetBool(allowNonDistFlag)
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	signOptions, err := signOptions(cmd)
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	sociOptions, err := sociOptions(cmd)
	if err != nil {
		return types.ImagePushOptions{}, err
	}
	return types.ImagePushOptions{
		GOptions:                       globalOptions,
		SignOptions:                    signOptions,
		SociOptions:                    sociOptions,
		Platforms:                      platform,
		AllPlatforms:                   allPlatforms,
		Estargz:                        estargz,
		IpfsEnsureImage:                ipfsEnsureImage,
		IpfsAddress:                    ipfsAddress,
		Quiet:                          quiet,
		AllowNondistributableArtifacts: allowNonDist,
		Stdout:                         cmd.OutOrStdout(),
	}, nil
}

func pushAction(cmd *cobra.Command, args []string) error {
	options, err := pushOptions(cmd)
	if err != nil {
		return err
	}
	rawRef := args[0]

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return image.Push(ctx, client, rawRef, options)
}

func pushShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ImageNames(cmd)
}

func signOptions(cmd *cobra.Command) (opt types.ImageSignOptions, err error) {
	if opt.Provider, err = cmd.Flags().GetString("sign"); err != nil {
		return
	}
	if opt.CosignKey, err = cmd.Flags().GetString("cosign-key"); err != nil {
		return
	}
	if opt.NotationKeyName, err = cmd.Flags().GetString("notation-key-name"); err != nil {
		return
	}
	return
}

func sociOptions(cmd *cobra.Command) (opt types.SociOptions, err error) {
	if opt.SpanSize, err = cmd.Flags().GetInt64("soci-span-size"); err != nil {
		return
	}
	if opt.MinLayerSize, err = cmd.Flags().GetInt64("soci-min-layer-size"); err != nil {
		return
	}
	return
}
