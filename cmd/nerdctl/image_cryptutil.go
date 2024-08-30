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

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
)

// registerImgcryptFlags register flags that correspond to parseImgcryptFlags().
// Platform flags are registered too.
//
// From:
// - https://github.com/containerd/imgcrypt/blob/v1.1.2/cmd/ctr/commands/flags/flags.go#L23-L44 (except skip-decrypt-auth)
// - https://github.com/containerd/imgcrypt/blob/v1.1.2/cmd/ctr/commands/images/encrypt.go#L52-L55
func registerImgcryptFlags(cmd *cobra.Command, encrypt bool) {
	flags := cmd.Flags()

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	flags.StringSlice("platform", []string{}, "Convert content for a specific platform")
	cmd.RegisterFlagCompletionFunc("platform", completion.Platforms)
	flags.Bool("all-platforms", false, "Convert content for all platforms")
	// #endregion

	flags.String("gpg-homedir", "", "The GPG homedir to use; by default gpg uses ~/.gnupg")
	flags.String("gpg-version", "", "The GPG version (\"v1\" or \"v2\"), default will make an educated guess")
	flags.StringSlice("key", []string{}, "A secret key's filename and an optional password separated by colon; this option may be provided multiple times")
	// While --recipient can be specified only for `nerdctl image encrypt`,
	// --dec-recipient can be specified for both `nerdctl image encrypt` and `nerdctl image decrypt`.
	flags.StringSlice("dec-recipient", []string{}, "Recipient of the image; used only for PKCS7 and must be an x509 certificate")

	if encrypt {
		// recipient is defined as StringSlice, not StringArray, to allow specifying "--recipient=jwe:FILE1,jwe:FILE2"
		flags.StringSlice("recipient", []string{}, "Recipient of the image is the person who can decrypt it in the form specified above (i.e. jwe:/path/to/pubkey)")
	}
}

func processImgCryptOptions(cmd *cobra.Command, args []string, encrypt bool) (types.ImageCryptOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ImageCryptOptions{}, err
	}
	platforms, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return types.ImageCryptOptions{}, err
	}
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return types.ImageCryptOptions{}, err
	}
	gpgHomeDir, err := cmd.Flags().GetString("gpg-homedir")
	if err != nil {
		return types.ImageCryptOptions{}, err
	}
	gpgVersion, err := cmd.Flags().GetString("gpg-version")
	if err != nil {
		return types.ImageCryptOptions{}, err
	}
	keys, err := cmd.Flags().GetStringSlice("key")
	if err != nil {
		return types.ImageCryptOptions{}, err
	}
	decRecipients, err := cmd.Flags().GetStringSlice("dec-recipient")
	if err != nil {
		return types.ImageCryptOptions{}, err
	}
	var recipients []string
	if encrypt {
		recipients, err = cmd.Flags().GetStringSlice("recipient")
		if err != nil {
			return types.ImageCryptOptions{}, err
		}
	}
	return types.ImageCryptOptions{
		GOptions:      globalOptions,
		Platforms:     platforms,
		AllPlatforms:  allPlatforms,
		GpgHomeDir:    gpgHomeDir,
		GpgVersion:    gpgVersion,
		Keys:          keys,
		DecRecipients: decRecipients,
		Recipients:    recipients,
		Stdout:        cmd.OutOrStdout(),
	}, nil
}

func getImgcryptAction(encrypt bool) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		options, err := processImgCryptOptions(cmd, args, encrypt)
		if err != nil {
			return err
		}
		srcRawRef := args[0]
		targetRawRef := args[1]

		client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
		if err != nil {
			return err
		}
		defer cancel()

		return image.Crypt(ctx, client, srcRawRef, targetRawRef, encrypt, options)
	}
}

func imgcryptShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ImageNames(cmd)
}
