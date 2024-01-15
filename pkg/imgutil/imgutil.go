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
	"fmt"
	"io"
	"reflect"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/imgcrypt"
	"github.com/containerd/imgcrypt/images/encryption"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/pull"
	"github.com/docker/docker/errdefs"
	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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
	imagewalker := &imagewalker.ImageWalker{
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
	count, err := imagewalker.Walk(ctx, rawRef)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, errdefs.NotFound(fmt.Errorf("got count 0 after walking"))
	}
	if res == nil {
		return nil, errdefs.NotFound(fmt.Errorf("got nil res after walking"))
	}
	return res, nil
}

// EnsureImage ensures the image.
//
// # When insecure is set, skips verifying certs, and also falls back to HTTP when the registry does not speak HTTPS
//
// FIXME: this func has too many args
func EnsureImage(ctx context.Context, client *containerd.Client, stdout, stderr io.Writer, snapshotter, rawRef string, mode PullMode, insecure bool, hostsDirs []string, ocispecPlatforms []ocispec.Platform, unpack *bool, quiet bool, rFlags RemoteSnapshotterFlags) (*EnsuredImage, error) {
	switch mode {
	case "always", "missing", "never":
		// NOP
	default:
		return nil, fmt.Errorf("unexpected pull mode: %q", mode)
	}

	// if not `always` pull and given one platform and image found locally, return existing image directly.
	if mode != "always" && len(ocispecPlatforms) == 1 {
		if res, err := GetExistingImage(ctx, client, snapshotter, rawRef, ocispecPlatforms[0]); err == nil {
			return res, nil
		} else if !errdefs.IsNotFound(err) {
			return nil, err
		}
	}

	if mode == "never" {
		return nil, fmt.Errorf("image not available: %q", rawRef)
	}

	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}
	ref := named.String()
	refDomain := refdocker.Domain(named)

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", refDomain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(hostsDirs))
	resolver, err := dockerconfigresolver.New(ctx, refDomain, dOpts...)
	if err != nil {
		return nil, err
	}

	img, err := PullImage(ctx, client, stdout, stderr, snapshotter, resolver, ref, ocispecPlatforms, unpack, quiet, rFlags)
	if err != nil {
		// In some circumstance (e.g. people just use 80 port to support pure http), the error will contain message like "dial tcp <port>: connection refused".
		if !errutil.IsErrHTTPResponseToHTTPSClient(err) && !errutil.IsErrConnectionRefused(err) {
			return nil, err
		}
		if insecure {
			log.G(ctx).WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", refDomain)
			dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
			resolver, err = dockerconfigresolver.New(ctx, refDomain, dOpts...)
			if err != nil {
				return nil, err
			}
			return PullImage(ctx, client, stdout, stderr, snapshotter, resolver, ref, ocispecPlatforms, unpack, quiet, rFlags)
		}
		log.G(ctx).WithError(err).Errorf("server %q does not seem to support HTTPS", refDomain)
		log.G(ctx).Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
		return nil, err

	}
	return img, nil
}

// ResolveDigest resolves `rawRef` and returns its descriptor digest.
func ResolveDigest(ctx context.Context, rawRef string, insecure bool, hostsDirs []string) (string, error) {
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return "", err
	}
	ref := named.String()
	refDomain := refdocker.Domain(named)

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", refDomain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(hostsDirs))
	resolver, err := dockerconfigresolver.New(ctx, refDomain, dOpts...)
	if err != nil {
		return "", err
	}

	_, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return "", err
	}

	return desc.Digest.String(), nil
}

// PullImage pulls an image using the specified resolver.
func PullImage(ctx context.Context, client *containerd.Client, stdout, stderr io.Writer, snapshotter string, resolver remotes.Resolver, ref string, ocispecPlatforms []ocispec.Platform, unpack *bool, quiet bool, rFlags RemoteSnapshotterFlags) (*EnsuredImage, error) {
	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(ctx)

	var containerdImage containerd.Image
	config := &pull.Config{
		Resolver:   resolver,
		RemoteOpts: []containerd.RemoteOpt{},
		Platforms:  ocispecPlatforms, // empty for all-platforms
	}
	if !quiet {
		config.ProgressOutput = stderr
	}

	// unpack(B) if given 1 platform unless specified by `unpack`
	unpackB := len(ocispecPlatforms) == 1
	if unpack != nil {
		unpackB = *unpack
		if unpackB && len(ocispecPlatforms) != 1 {
			return nil, fmt.Errorf("unpacking requires a single platform to be specified (e.g., --platform=amd64)")
		}
	}

	snOpt := getSnapshotterOpts(snapshotter)
	if unpackB {
		log.G(ctx).Debugf("The image will be unpacked for platform %q, snapshotter %q.", ocispecPlatforms[0], snapshotter)
		imgcryptPayload := imgcrypt.Payload{}
		imgcryptUnpackOpt := encryption.WithUnpackConfigApplyOpts(encryption.WithDecryptedUnpack(&imgcryptPayload))
		config.RemoteOpts = append(config.RemoteOpts,
			containerd.WithPullUnpack,
			containerd.WithUnpackOpts([]containerd.UnpackOpt{imgcryptUnpackOpt}))

		// different remote snapshotters will update pull.Config separately
		snOpt.apply(config, ref, rFlags)
	} else {
		log.G(ctx).Debugf("The image will not be unpacked. Platforms=%v.", ocispecPlatforms)
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
		Snapshotter: snapshotter,
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
	return config, configDesc, nil
}

// ParseRepoTag parses raw `imgName` to repository and tag.
func ParseRepoTag(imgName string) (string, string) {
	log.L.Debugf("raw image name=%q", imgName)

	ref, err := refdocker.ParseDockerRef(imgName)
	if err != nil {
		log.L.WithError(err).Debugf("unparsable image name %q", imgName)
		return "", ""
	}

	var tag string

	if tagged, ok := ref.(refdocker.Tagged); ok {
		tag = tagged.Tag()
	}
	repository := refdocker.FamiliarName(ref)

	return repository, tag
}

type snapshotKey string

// recursive function to calculate total usage of key's parent
func (key snapshotKey) add(ctx context.Context, s snapshots.Snapshotter, usage *snapshots.Usage) error {
	if key == "" {
		return nil
	}
	u, err := s.Usage(ctx, string(key))
	if err != nil {
		return err
	}

	usage.Add(u)

	info, err := s.Stat(ctx, string(key))
	if err != nil {
		return err
	}

	key = snapshotKey(info.Parent)
	return key.add(ctx, s, usage)
}

// UnpackedImageSize is the size of the unpacked snapshots.
// Does not contain the size of the blobs in the content store. (Corresponds to Docker).
func UnpackedImageSize(ctx context.Context, s snapshots.Snapshotter, img containerd.Image) (int64, error) {
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return 0, err
	}

	chainID := identity.ChainID(diffIDs).String()
	usage, err := s.Usage(ctx, chainID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			log.G(ctx).WithError(err).Debugf("image %q seems not unpacked", img.Name())
			return 0, nil
		}
		return 0, err
	}

	info, err := s.Stat(ctx, chainID)
	if err != nil {
		return 0, err
	}

	//add ChainID's parent usage to the total usage
	if err := snapshotKey(info.Parent).add(ctx, s, &usage); err != nil {
		return 0, err
	}
	return usage.Size, nil
}
