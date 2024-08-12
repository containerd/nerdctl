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

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/imgcrypt/images/encryption"
	"github.com/containerd/imgcrypt/images/encryption/parsehelpers"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Crypt(ctx context.Context, client *containerd.Client, srcRawRef, targetRawRef string, encrypt bool, options types.ImageCryptOptions) error {
	var convertOpts = []converter.Opt{}
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

	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platforms)
	if err != nil {
		return err
	}
	convertOpts = append(convertOpts, converter.WithPlatform(platMC))

	imgcryptFlags, err := parseImgcryptFlags(options, encrypt)
	if err != nil {
		return err
	}

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
	fmt.Fprintln(options.Stdout, newImg.Target.Digest.String())
	return nil
}

// parseImgcryptFlags corresponds to https://github.com/containerd/imgcrypt/blob/v1.1.2/cmd/ctr/commands/images/crypt_utils.go#L244-L252
func parseImgcryptFlags(options types.ImageCryptOptions, encrypt bool) (parsehelpers.EncArgs, error) {
	var a parsehelpers.EncArgs

	a.GPGHomedir = options.GpgHomeDir
	a.GPGVersion = options.GpgVersion
	a.Key = options.Keys
	if encrypt {
		a.Recipient = options.Recipients
		if len(a.Recipient) == 0 {
			return a, errors.New("at least one recipient must be specified (e.g., --recipient=jwe:mypubkey.pem)")
		}
	}
	// While --recipient can be specified only for `nerdctl image encrypt`,
	// --dec-recipient can be specified for both `nerdctl image encrypt` and `nerdctl image decrypt`.
	a.DecRecipient = options.DecRecipients
	return a, nil
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
