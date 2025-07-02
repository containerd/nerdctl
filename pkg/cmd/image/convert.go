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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/containerd/v2/core/images/converter/uncompress"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	converterutil "github.com/containerd/nerdctl/v2/pkg/imgutil/converter"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Convert(ctx context.Context, client *containerd.Client, srcRawRef, targetRawRef string, options types.ImageConvertOptions) error {
	var (
		convertOpts = []converter.Opt{}
	)
	if srcRawRef == "" || targetRawRef == "" {
		return errors.New("src and target image need to be specified")
	}

	parsedReference, err := referenceutil.Parse(srcRawRef)
	if err != nil {
		return err
	}
	srcRef := parsedReference.String()

	parsedReference, err = referenceutil.Parse(targetRawRef)
	if err != nil {
		return err
	}
	targetRef := parsedReference.String()

	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platforms)
	if err != nil {
		return err
	}
	convertOpts = append(convertOpts, converter.WithPlatform(platMC))

	// Ensure all the layers are here: https://github.com/containerd/nerdctl/issues/3425
	err = EnsureAllContent(ctx, client, srcRef, platMC, options.GOptions)
	if err != nil {
		return err
	}

	estargz := options.Estargz
	zstd := options.Zstd
	zstdchunked := options.ZstdChunked
	overlaybd := options.Overlaybd
	nydus := options.Nydus
	var finalize func(ctx context.Context, cs content.Store, ref string, desc *ocispec.Descriptor) (*images.Image, error)
	if estargz || zstd || zstdchunked || overlaybd || nydus {
		convertCount := 0
		if estargz {
			convertCount++
		}
		if zstd {
			convertCount++
		}
		if zstdchunked {
			convertCount++
		}
		if overlaybd {
			convertCount++
		}
		if nydus {
			convertCount++
		}

		if convertCount > 1 {
			return errors.New("options --estargz, --zstdchunked, --overlaybd and --nydus lead to conflict, only one of them can be used")
		}

		var convertFunc converter.ConvertFunc
		var convertType string
		switch {
		case estargz:
			convertFunc, finalize, err = getEstargzConvertFunc(options)
			if err != nil {
				return err
			}
			convertType = "estargz"
		case zstd:
			convertFunc, err = getZstdConverter(options)
			if err != nil {
				return err
			}
			convertType = "zstd"
		case zstdchunked:
			convertFunc, err = getZstdChunkedConvertFunc(options)
			if err != nil {
				return err
			}
			convertType = "zstdchunked"
		case overlaybd:
			convertOpts, err = addOverlayBDConverterOpts(ctx, client, srcRef, options, convertOpts)
			if err != nil {
				return err
			}
			convertType = "overlaybd"
		case nydus:
			convertOpts, err = addNydusConverterOpts(ctx, client, options, convertOpts, platMC)
			if err != nil {
				return err
			}
			convertType = "nydus"
		}

		if convertType != "overlaybd" {
			convertOpts = append(convertOpts, converter.WithLayerConvertFunc(convertFunc))
		}
		if !options.Oci {
			if nydus || overlaybd {
				log.G(ctx).Warnf("option --%s should be used in conjunction with --oci, forcibly enabling on oci mediatype for %s conversion", convertType, convertType)
			} else {
				log.G(ctx).Warnf("option --%s should be used in conjunction with --oci", convertType)
			}
		}
		if options.Uncompress {
			return fmt.Errorf("option --%s conflicts with --uncompress", convertType)
		}
	}

	if options.Uncompress {
		convertOpts = append(convertOpts, converter.WithLayerConvertFunc(uncompress.LayerConvertFunc))
	}

	if options.Oci {
		convertOpts = append(convertOpts, converter.WithDockerToOCI(true))
	}

	// converter.Convert() gains the lease by itself
	newImg, err := converterutil.Convert(ctx, client, targetRef, srcRef, convertOpts...)
	if err != nil {
		return err
	}
	res := converterutil.ConvertedImageInfo{
		Image: newImg.Name + "@" + newImg.Target.Digest.String(),
	}
	if finalize != nil {
		ctx, done, err := client.WithLease(ctx)
		if err != nil {
			return err
		}
		defer done(ctx)
		newI, err := finalize(ctx, client.ContentStore(), targetRef, &newImg.Target)
		if err != nil {
			return err
		}
		is := client.ImageService()
		finimg, err := is.Update(ctx, *newI)
		if err != nil {
			return err
		}
		res.ExtraImages = append(res.ExtraImages, finimg.Name+"@"+finimg.Target.Digest.String())
	}
	return printConvertedImage(options.Stdout, options, res)
}

func getZstdConverter(options types.ImageConvertOptions) (converter.ConvertFunc, error) {
	return converterutil.ZstdLayerConvertFunc(options)
}

func printConvertedImage(stdout io.Writer, options types.ImageConvertOptions, img converterutil.ConvertedImageInfo) error {
	switch options.Format {
	case "json":
		b, err := json.MarshalIndent(img, "", "    ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(b))
	default:
		for i, e := range img.ExtraImages {
			elems := strings.SplitN(e, "@", 2)
			if len(elems) < 2 {
				log.L.Errorf("extra reference %q doesn't contain digest", e)
			} else {
				log.L.Infof("Extra image(%d) %s", i, elems[0])
			}
		}
		elems := strings.SplitN(img.Image, "@", 2)
		if len(elems) < 2 {
			log.L.Errorf("reference %q doesn't contain digest", img.Image)
		} else {
			fmt.Fprintln(stdout, elems[1])
		}
	}
	return nil
}
