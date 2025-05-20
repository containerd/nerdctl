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

package imgutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/errdefs"
	"github.com/containerd/imgcrypt/v2"
	"github.com/containerd/imgcrypt/v2/images/encryption"
	"github.com/containerd/log"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/pull"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

// EnsuredImage contains the image existed in containerd and its metadata.
type EnsuredImage struct {
	Ref         string
	Image       containerd.Image
	ImageConfig ocispec.ImageConfig
	Snapshotter string
	Remote      bool // true for stargz or overlaybd
}

// PullMode is either one of "always", "missing", "never"
type PullMode = string

// GetExistingImage returns the specified image if exists in containerd. Return errdefs.NotFound() if not exists.
func GetExistingImage(ctx context.Context, client *containerd.Client, snapshotter, rawRef string, platform ocispec.Platform) (*EnsuredImage, error) {
	var res *EnsuredImage
	imgwalker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if res != nil {
				return nil
			}
			image := containerd.NewImageWithPlatform(client, found.Image, platforms.OnlyStrict(platform))
			imgConfig, err := getImageConfig(ctx, image)
			if err != nil {
				// Image found but blob not found for foreign arch
				// Ignore err and return nil, so that the walker can visit the next candidate.
				return nil
			}
			res = &EnsuredImage{
				Ref:         found.Image.Name,
				Image:       image,
				ImageConfig: *imgConfig,
				Snapshotter: snapshotter,
				Remote:      getSnapshotterOpts(snapshotter).isRemote(),
			}
			if unpacked, err := image.IsUnpacked(ctx, snapshotter); err == nil && !unpacked {
				if err := image.Unpack(ctx, snapshotter); err != nil {
					return err
				}
			}
			return nil
		},
	}
	count, err := imgwalker.Walk(ctx, rawRef)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, errors.Join(errdefs.ErrNotFound, errors.New("got count 0 after walking"))
	}
	if res == nil {
		return nil, errors.Join(errdefs.ErrNotFound, errors.New("got nil res after walking"))
	}
	return res, nil
}

// EnsureImage ensures the image.
//
// # When insecure is set, skips verifying certs, and also falls back to HTTP when the registry does not speak HTTPS
func EnsureImage(ctx context.Context, client *containerd.Client, rawRef string, options types.ImagePullOptions) (*EnsuredImage, error) {
	switch options.Mode {
	case "always", "missing", "never":
		// NOP
	default:
		return nil, fmt.Errorf("unexpected pull mode: %q", options.Mode)
	}

	// if not `always` pull and given one platform and image found locally, return existing image directly.
	if options.Mode != "always" && len(options.OCISpecPlatform) == 1 {
		if res, err := GetExistingImage(ctx, client, options.GOptions.Snapshotter, rawRef, options.OCISpecPlatform[0]); err == nil {
			return res, nil
		} else if !errdefs.IsNotFound(err) {
			return nil, err
		}
	}

	if options.Mode == "never" {
		return nil, fmt.Errorf("image not available: %q", rawRef)
	}

	parsedReference, err := referenceutil.Parse(rawRef)
	if err != nil {
		return nil, err
	}

	var dOpts []dockerconfigresolver.Opt
	if options.GOptions.InsecureRegistry {
		log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", parsedReference.Domain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(options.GOptions.HostsDir))
	resolver, err := dockerconfigresolver.New(ctx, parsedReference.Domain, dOpts...)
	if err != nil {
		return nil, err
	}

	img, err := PullImage(ctx, client, resolver, parsedReference.String(), options)
	if err != nil {
		// In some circumstance (e.g. people just use 80 port to support pure http), the error will contain message like "dial tcp <port>: connection refused".
		if !errors.Is(err, http.ErrSchemeMismatch) && !errutil.IsErrConnectionRefused(err) {
			return nil, err
		}
		if options.GOptions.InsecureRegistry {
			log.G(ctx).WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", parsedReference.Domain)
			dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
			resolver, err = dockerconfigresolver.New(ctx, parsedReference.Domain, dOpts...)
			if err != nil {
				return nil, err
			}
			return PullImage(ctx, client, resolver, parsedReference.String(), options)
		}
		log.G(ctx).WithError(err).Errorf("server %q does not seem to support HTTPS", parsedReference.Domain)
		log.G(ctx).Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
		return nil, err

	}
	return img, nil
}

// ResolveDigest resolves `rawRef` and returns its descriptor digest.
func ResolveDigest(ctx context.Context, rawRef string, insecure bool, hostsDirs []string) (string, error) {
	parsedReference, err := referenceutil.Parse(rawRef)
	if err != nil {
		return "", err
	}

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", parsedReference.Domain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(hostsDirs))
	resolver, err := dockerconfigresolver.New(ctx, parsedReference.Domain, dOpts...)
	if err != nil {
		return "", err
	}

	_, desc, err := resolver.Resolve(ctx, parsedReference.String())
	if err != nil {
		return "", err
	}

	return desc.Digest.String(), nil
}

// PullImage pulls an image using the specified resolver.
func PullImage(ctx context.Context, client *containerd.Client, resolver remotes.Resolver, ref string, options types.ImagePullOptions) (*EnsuredImage, error) {
	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(ctx)

	var containerdImage containerd.Image
	config := &pull.Config{
		Resolver:   resolver,
		RemoteOpts: []containerd.RemoteOpt{},
		Platforms:  options.OCISpecPlatform, // empty for all-platforms
	}
	if !options.Quiet {
		config.ProgressOutput = options.Stderr
		if options.ProgressOutputToStdout {
			config.ProgressOutput = options.Stdout
		}
	}

	// unpack(B) if given 1 platform unless specified by `unpack`
	unpackB := len(options.OCISpecPlatform) == 1
	if options.Unpack != nil {
		unpackB = *options.Unpack
		if unpackB && len(options.OCISpecPlatform) != 1 {
			return nil, fmt.Errorf("unpacking requires a single platform to be specified (e.g., --platform=amd64)")
		}
	}

	snOpt := getSnapshotterOpts(options.GOptions.Snapshotter)
	if unpackB {
		log.G(ctx).Debugf("The image will be unpacked for platform %q, snapshotter %q.", options.OCISpecPlatform[0], options.GOptions.Snapshotter)
		imgcryptPayload := imgcrypt.Payload{}
		imgcryptUnpackOpt := encryption.WithUnpackConfigApplyOpts(encryption.WithDecryptedUnpack(&imgcryptPayload))
		config.RemoteOpts = append(config.RemoteOpts,
			containerd.WithPullUnpack,
			containerd.WithUnpackOpts([]containerd.UnpackOpt{imgcryptUnpackOpt}))

		// different remote snapshotters will update pull.Config separately
		snOpt.apply(config, ref, options.RFlags)
	} else {
		log.G(ctx).Debugf("The image will not be unpacked. Platforms=%v.", options.OCISpecPlatform)
	}

	containerdImage, err = pull.Pull(ctx, client, ref, config)
	if err != nil {
		return nil, err
	}
	imgConfig, err := getImageConfig(ctx, containerdImage)
	if err != nil {
		return nil, err
	}
	res := &EnsuredImage{
		Ref:         ref,
		Image:       containerdImage,
		ImageConfig: *imgConfig,
		Snapshotter: options.GOptions.Snapshotter,
		Remote:      snOpt.isRemote(),
	}
	return res, nil

}

func getImageConfig(ctx context.Context, image containerd.Image) (*ocispec.ImageConfig, error) {
	desc, err := image.Config(ctx)
	if err != nil {
		return nil, err
	}
	switch desc.MediaType {
	case ocispec.MediaTypeImageConfig, images.MediaTypeDockerSchema2Config:
		var ocispecImage ocispec.Image
		b, err := content.ReadBlob(ctx, image.ContentStore(), desc)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(b, &ocispecImage); err != nil {
			return nil, err
		}

		if err := addHealthCheckToImageConfig(b, &ocispecImage.Config); err != nil {
			log.G(ctx).WithError(err).Debug("failed to add health check config")
		}
		return &ocispecImage.Config, nil
	default:
		return nil, fmt.Errorf("unknown media type %q", desc.MediaType)
	}
}

// ReadIndex returns image index, or nil for non-indexed image.
func ReadIndex(ctx context.Context, img containerd.Image) (*ocispec.Index, *ocispec.Descriptor, error) {
	desc := img.Target()
	if !images.IsIndexType(desc.MediaType) {
		return nil, nil, nil
	}
	b, err := content.ReadBlob(ctx, img.ContentStore(), desc)
	if err != nil {
		return nil, &desc, err
	}
	var idx ocispec.Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, &desc, err
	}

	return &idx, &desc, nil
}

// ReadManifest returns the manifest for img.platform, or nil if no manifest was found.
func ReadManifest(ctx context.Context, img containerd.Image) (*ocispec.Manifest, *ocispec.Descriptor, error) {
	cs := img.ContentStore()
	targetDesc := img.Target()
	if images.IsManifestType(targetDesc.MediaType) {
		b, err := content.ReadBlob(ctx, img.ContentStore(), targetDesc)
		if err != nil {
			return nil, &targetDesc, err
		}
		var mani ocispec.Manifest
		if err := json.Unmarshal(b, &mani); err != nil {
			return nil, &targetDesc, err
		}
		return &mani, &targetDesc, nil
	}
	if images.IsIndexType(targetDesc.MediaType) {
		idx, _, err := ReadIndex(ctx, img)
		if err != nil {
			return nil, nil, err
		}
		configDesc, err := img.Config(ctx) // aware of img.platform
		if err != nil {
			return nil, nil, err
		}
		// We can't access the private `img.platform` variable.
		// So, we find the manifest object by comparing the config desc.
		for _, maniDesc := range idx.Manifests {
			maniDesc := maniDesc
			// ignore non-nil err
			if b, err := content.ReadBlob(ctx, cs, maniDesc); err == nil {
				var mani ocispec.Manifest
				if err := json.Unmarshal(b, &mani); err != nil {
					return nil, nil, err
				}
				if reflect.DeepEqual(configDesc, mani.Config) {
					return &mani, &maniDesc, nil
				}
			}
		}
	}
	// no manifest was found
	return nil, nil, nil
}

// ReadImageConfig reads the config spec (`application/vnd.oci.image.config.v1+json`) for img.platform from content store.
func ReadImageConfig(ctx context.Context, img containerd.Image) (ocispec.Image, ocispec.Descriptor, error) {
	var config ocispec.Image

	configDesc, err := img.Config(ctx) // aware of img.platform
	if err != nil {
		return config, configDesc, err
	}
	p, err := content.ReadBlob(ctx, img.ContentStore(), configDesc)
	if err != nil {
		return config, configDesc, err
	}
	if err := json.Unmarshal(p, &config); err != nil {
		return config, configDesc, err
	}
	if err := addHealthCheckToImageConfig(p, &config.Config); err != nil {
		log.G(ctx).WithError(err).Debug("failed to add health check config")
	}
	return config, configDesc, nil
}

// ParseRepoTag parses raw `imgName` to repository and tag.
func ParseRepoTag(imgName string) (string, string) {
	log.L.Debugf("raw image name=%q", imgName)

	parsedReference, err := referenceutil.Parse(imgName)
	if err != nil {
		log.L.WithError(err).Debugf("unparsable image name %q", imgName)
		return "", ""
	}

	return parsedReference.FamiliarName(), parsedReference.Tag
}

// ResourceUsage will return:
// - the Usage value of the resource referenced by ID
// - the cumulative Usage value of the resource, and all parents, recursively
// Typically, for a running container, this will equal the size of the read-write layer, plus the sum of the size of all layers in the base image
func ResourceUsage(ctx context.Context, snapshotter snapshots.Snapshotter, resourceID string) (snapshots.Usage, snapshots.Usage, error) {
	first := snapshots.Usage{}
	total := snapshots.Usage{}
	var info snapshots.Info
	for next := resourceID; next != ""; next = info.Parent {
		// Get the resource usage info
		usage, err := snapshotter.Usage(ctx, next)
		if err != nil {
			return first, total, err
		}
		// In case that's the first one, store that
		if next == resourceID {
			first = usage
		}
		// And increment totals
		total.Size += usage.Size
		total.Inodes += usage.Inodes

		// Now, get the parent, if any and iterate
		info, err = snapshotter.Stat(ctx, next)
		if err != nil {
			return first, total, err
		}
	}

	return first, total, nil
}

// UnpackedImageSize is the size of the unpacked snapshots.
// Does not contain the size of the blobs in the content store. (Corresponds to Docker).
func UnpackedImageSize(ctx context.Context, s snapshots.Snapshotter, img containerd.Image) (int64, error) {
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return 0, err
	}

	chainID := identity.ChainID(diffIDs).String()
	_, total, err := ResourceUsage(ctx, s, chainID)

	return total.Size, err
}

// GetUnusedImages returns the list of all images which are not referenced by a container.
func GetUnusedImages(ctx context.Context, client *containerd.Client, filters ...Filter) ([]images.Image, error) {
	var (
		imageStore     = client.ImageService()
		containerStore = client.ContainerService()
	)

	containers, err := containerStore.List(ctx)
	if err != nil {
		return []images.Image{}, err
	}

	usedImages := make(map[string]struct{})
	for _, container := range containers {
		usedImages[container.Image] = struct{}{}
	}

	allImages, err := imageStore.List(ctx)
	if err != nil {
		return []images.Image{}, err
	}

	unusedImages := make([]images.Image, 0, len(allImages))
	for _, image := range allImages {
		if _, ok := usedImages[image.Name]; ok {
			continue
		}
		unusedImages = append(unusedImages, image)
	}

	return ApplyFilters(unusedImages, filters...)
}

// GetDanglingImages returns the list of all images which are not tagged.
func GetDanglingImages(ctx context.Context, client *containerd.Client, filters ...Filter) ([]images.Image, error) {
	var (
		imageStore = client.ImageService()
	)

	allImages, err := imageStore.List(ctx)
	if err != nil {
		return []images.Image{}, err
	}

	filters = append([]Filter{FilterDanglingImages()}, filters...)

	return ApplyFilters(allImages, filters...)
}

// addHealthCheckToImageConfig extracts health check information from the image content store and adds it to the labels
func addHealthCheckToImageConfig(rawConfigContent []byte, config *ocispec.ImageConfig) error {
	var imgConfig struct {
		Config struct {
			Healthcheck *healthcheck.Healthcheck `json:"Healthcheck,omitempty"`
		} `json:"config"`
	}

	if err := json.Unmarshal(rawConfigContent, &imgConfig); err != nil {
		return err
	}

	if imgConfig.Config.Healthcheck != nil {
		healthCheckJSON, err := json.Marshal(imgConfig.Config.Healthcheck)
		if err != nil {
			return err
		}
		if config.Labels == nil {
			config.Labels = make(map[string]string)
		}
		config.Labels[labels.HealthCheck] = string(healthCheckJSON)
	}
	return nil
}
