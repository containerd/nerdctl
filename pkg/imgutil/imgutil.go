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
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/imgcrypt"
	"github.com/containerd/imgcrypt/images/encryption"
	"github.com/containerd/nerdctl/pkg/errutil"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/pkg/imgutil/pull"
	"github.com/containerd/stargz-snapshotter/fs/source"
	"github.com/docker/docker/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/sirupsen/logrus"
)

type EnsuredImage struct {
	Ref         string
	Image       containerd.Image
	ImageConfig ocispec.ImageConfig
	Snapshotter string
	Remote      bool // true for stargz or overlaybd
}

// PullMode is either one of "always", "missing", "never"
type PullMode = string

// GetExistingImage returns the specified image if exists in containerd. May return errdefs.NotFound() if not exists.
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
				Remote:      isStargz(snapshotter) || isOverlaybd(snapshotter),
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
// When insecure is set, skips verifying certs, and also falls back to HTTP when the registry does not speak HTTPS
//
// FIXME: this func has too many args
func EnsureImage(ctx context.Context, client *containerd.Client, stdout, stderr io.Writer, snapshotter, rawRef string, mode PullMode, insecure bool, hostsDirs []string, ocispecPlatforms []ocispec.Platform, unpack *bool, quiet bool) (*EnsuredImage, error) {
	switch mode {
	case "always", "missing", "never":
		// NOP
	default:
		return nil, fmt.Errorf("unexpected pull mode: %q", mode)
	}
	if mode != "always" && len(ocispecPlatforms) == 1 {
		res, err := GetExistingImage(ctx, client, snapshotter, rawRef, ocispecPlatforms[0])
		if err == nil {
			return res, nil
		}
		if !errdefs.IsNotFound(err) {
			return nil, err
		}
	}

	if mode == "never" {
		return nil, fmt.Errorf("image %q is not available", rawRef)
	}

	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}
	ref := named.String()
	refDomain := refdocker.Domain(named)

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		logrus.Warnf("skipping verifying HTTPS certs for %q", refDomain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(hostsDirs))
	resolver, err := dockerconfigresolver.New(ctx, refDomain, dOpts...)
	if err != nil {
		return nil, err
	}

	img, err := PullImage(ctx, client, stdout, stderr, snapshotter, resolver, ref, ocispecPlatforms, unpack, quiet)
	if err != nil {
		// In some circumstance (e.g. people just use 80 port to support pure http), the error will contain message like "dial tcp <port>: connection refused".
		if !errutil.IsErrHTTPResponseToHTTPSClient(err) && !errutil.IsErrConnectionRefused(err) {
			return nil, err
		}
		if insecure {
			logrus.WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", refDomain)
			dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
			resolver, err = dockerconfigresolver.New(ctx, refDomain, dOpts...)
			if err != nil {
				return nil, err
			}
			return PullImage(ctx, client, stdout, stderr, snapshotter, resolver, ref, ocispecPlatforms, unpack, quiet)
		} else {
			logrus.WithError(err).Errorf("server %q does not seem to support HTTPS", refDomain)
			logrus.Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
			return nil, err
		}
	}
	return img, nil
}

func ResolveDigest(ctx context.Context, rawRef string, insecure bool, hostsDirs []string) (string, error) {
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return "", err
	}
	ref := named.String()
	refDomain := refdocker.Domain(named)

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		logrus.Warnf("skipping verifying HTTPS certs for %q", refDomain)
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
func PullImage(ctx context.Context, client *containerd.Client, stdout, stderr io.Writer, snapshotter string, resolver remotes.Resolver, ref string, ocispecPlatforms []ocispec.Platform, unpack *bool, quiet bool) (*EnsuredImage, error) {
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

	var unpackB bool
	if unpack != nil {
		unpackB = *unpack
		if unpackB && len(ocispecPlatforms) != 1 {
			return nil, fmt.Errorf("unpacking requires a single platform to be specified (e.g., --platform=amd64)")
		}
	} else {
		unpackB = len(ocispecPlatforms) == 1
	}

	var sgz, overlaybd bool
	if unpackB {
		logrus.Debugf("The image will be unpacked for platform %q, snapshotter %q.", ocispecPlatforms[0], snapshotter)
		imgcryptPayload := imgcrypt.Payload{}
		imgcryptUnpackOpt := encryption.WithUnpackConfigApplyOpts(encryption.WithDecryptedUnpack(&imgcryptPayload))
		config.RemoteOpts = append(config.RemoteOpts,
			containerd.WithPullUnpack,
			containerd.WithUnpackOpts([]containerd.UnpackOpt{imgcryptUnpackOpt}))

		sgz = isStargz(snapshotter)
		if sgz {
			// TODO: support "skip-content-verify"
			config.RemoteOpts = append(
				config.RemoteOpts,
				containerd.WithImageHandlerWrapper(source.AppendDefaultLabelsHandlerWrapper(ref, 10*1024*1024)),
			)
		}
		overlaybd = isOverlaybd(snapshotter)
		if overlaybd {
			snlabel := map[string]string{"containerd.io/snapshot/image-ref": ref}
			logrus.Debugf("append remote opts: %s", snlabel)

			config.RemoteOpts = append(
				config.RemoteOpts,
				containerd.WithPullSnapshotter(snapshotter, snapshots.WithLabels(snlabel)),
			)
		} else {
			config.RemoteOpts = append(
				config.RemoteOpts,
				containerd.WithPullSnapshotter(snapshotter))
		}
	} else {
		logrus.Debugf("The image will not be unpacked. Platforms=%v.", ocispecPlatforms)
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
		Remote:      (sgz || overlaybd),
	}
	return res, nil

}

func isStargz(sn string) bool {
	if !strings.Contains(sn, "stargz") {
		return false
	}
	if sn != "stargz" {
		logrus.Debugf("assuming %q to be a stargz-compatible snapshotter", sn)
	}
	return true
}

func isOverlaybd(sn string) bool {
	return sn == "overlaybd"
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

// ReadIndex returns the index .
// ReadIndex returns nil for non-indexed image.
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

// ReadManifest returns the manifest for img.platform.
// ReadManifest returns nil if no manifest was found.
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

func ParseRepoTag(imgName string) (string, string) {
	logrus.Debugf("raw image name=%q", imgName)

	ref, err := refdocker.ParseDockerRef(imgName)
	if err != nil {
		logrus.WithError(err).Warnf("unparsable image name %q", imgName)
		return "", ""
	}

	var tag string

	if tagged, ok := ref.(refdocker.Tagged); ok {
		tag = tagged.Tag()
	}
	repository := refdocker.FamiliarName(ref)

	return repository, tag
}
