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
	"context"
	"errors"
	"fmt"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/imgcrypt/images/encryption"
	"github.com/containerd/imgcrypt/images/encryption/parsehelpers"
	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
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
	cmd.RegisterFlagCompletionFunc("platform", completion.ShellCompletePlatforms)
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

// parseImgcryptFlags corresponds to https://github.com/containerd/imgcrypt/blob/v1.1.2/cmd/ctr/commands/images/crypt_utils.go#L244-L252
func parseImgcryptFlags(cmd *cobra.Command, encrypt bool) (parsehelpers.EncArgs, error) {
	var err error
	flags := cmd.Flags()
	var a parsehelpers.EncArgs

	a.GPGHomedir, err = flags.GetString("gpg-homedir")
	if err != nil {
		return a, err
	}
	a.GPGVersion, err = flags.GetString("gpg-version")
	if err != nil {
		return a, err
	}
	a.Key, err = flags.GetStringSlice("key")
	if err != nil {
		return a, err
	}
	if encrypt {
		a.Recipient, err = flags.GetStringSlice("recipient")
		if err != nil {
			return a, err
		}
		if len(a.Recipient) == 0 {
			return a, errors.New("at least one recipient must be specified (e.g., --recipient=jwe:mypubkey.pem)")
		}
	}
	// While --recipient can be specified only for `nerdctl image encrypt`,
	// --dec-recipient can be specified for both `nerdctl image encrypt` and `nerdctl image decrypt`.
	a.DecRecipient, err = flags.GetStringSlice("dec-recipient")
	if err != nil {
		return a, err
	}
	return a, nil
}

func getImgcryptAction(encrypt bool) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		var convertOpts = []converter.Opt{}
		srcRawRef := args[0]
		targetRawRef := args[1]
		if srcRawRef == "" || targetRawRef == "" {
			return errors.New("src and target image need to be specified")
		}

		srcNamed, err := referenceutil.ParseAny(srcRawRef)
		if err != nil {
			return err
		}
		srcRef := srcNamed.String()

		targetNamed, err := referenceutil.ParseDockerRef(targetRawRef)
		if err != nil {
			return err
		}
		targetRef := targetNamed.String()

		allPlatforms, err := cmd.Flags().GetBool("all-platforms")
		if err != nil {
			return err
		}
		platform, err := cmd.Flags().GetStringSlice("platform")
		if err != nil {
			return err
		}
		platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
		if err != nil {
			return err
		}
		convertOpts = append(convertOpts, converter.WithPlatform(platMC))

		imgcryptFlags, err := parseImgcryptFlags(cmd, encrypt)
		if err != nil {
			return err
		}

		client, ctx, cancel, err := nerdClient.NewClient(cmd)
		if err != nil {
			return err
		}
		defer cancel()

		srcImg, err := client.ImageService().Get(ctx, srcRef)
		if err != nil {
			return err
		}
		layerDescs, err := platformutil.LayerDescs(ctx, client.ContentStore(), srcImg.Target, platMC)
		if err != nil {
			return err
		}
		layerFilter := func(desc ocispec.Descriptor) bool {
			return true
		}
		var convertFunc converter.ConvertFunc
		if encrypt {
			cc, err := parsehelpers.CreateCryptoConfig(imgcryptFlags, layerDescs)
			if err != nil {
				return err
			}
			convertFunc = encryption.GetImageEncryptConverter(&cc, layerFilter)
		} else {
			cc, err := parsehelpers.CreateDecryptCryptoConfig(imgcryptFlags, layerDescs)
			if err != nil {
				return err
			}
			convertFunc = encryption.GetImageDecryptConverter(&cc, layerFilter)
		}
		// we have to compose the DefaultIndexConvertFunc here to match platforms.
		convertFunc = composeConvertFunc(converter.DefaultIndexConvertFunc(nil, false, platMC), convertFunc)
		convertOpts = append(convertOpts, converter.WithIndexConvertFunc(convertFunc))

		// converter.Convert() gains the lease by itself
		newImg, err := converter.Convert(ctx, client, targetRef, srcRef, convertOpts...)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), newImg.Target.Digest.String())
		return nil
	}
}

func composeConvertFunc(a, b converter.ConvertFunc) converter.ConvertFunc {
	return func(ctx context.Context, cs content.Store, desc ocispec.Descriptor) (*ocispec.Descriptor, error) {
		newDesc, err := a(ctx, cs, desc)
		if err != nil {
			return newDesc, err
		}
		if newDesc == nil {
			return b(ctx, cs, desc)
		}
		return b(ctx, cs, *newDesc)
	}
}

func imgcryptShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ShellCompleteImageNames(cmd)
}
