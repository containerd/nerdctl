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

// Package pull forked from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/content/fetch.go
package pull

import (
	"context"
	"github.com/containerd/containerd/labels"
	"io"

	"github.com/containerd/containerd"
	ctrcontent "github.com/containerd/containerd/cmd/ctr/commands/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/nerdctl/pkg/platformutil"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Config for content fetch
type Config struct {
	// Resolver
	Resolver remotes.Resolver
	// ProgressOutput to display progress
	ProgressOutput io.Writer
	// RemoteOpts, e.g. containerd.WithPullUnpack.
	//
	// Regardless to RemoteOpts, the following opts are always set:
	// WithResolver, WithImageHandler, WithSchema1Conversion
	//
	// RemoteOpts related to unpacking can be set only when len(Platforms) is 1.
	RemoteOpts []containerd.RemoteOpt
	Platforms  []ocispec.Platform // empty for all-platforms
}

// Pull loads all resources into the content store and returns the image
func Pull(ctx context.Context, client *containerd.Client, ref string, config *Config) (containerd.Image, error) {
	ongoing := ctrcontent.NewJobs(ref)

	pctx, stopProgress := context.WithCancel(ctx)
	progress := make(chan struct{})

	go func() {
		if config.ProgressOutput != nil {
			// no progress bar, because it hides some debug logs
			ctrcontent.ShowProgress(pctx, ongoing, client.ContentStore(), config.ProgressOutput)
		}
		close(progress)
	}()

	h := images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		if desc.MediaType != images.MediaTypeDockerSchema1Manifest {
			ongoing.Add(desc)
		}
		return nil, nil
	})

	log.G(pctx).WithField("image", ref).Debug("fetching")
	platformMC := platformutil.NewMatchComparerFromOCISpecPlatformSlice(config.Platforms)
	opts := []containerd.RemoteOpt{
		containerd.WithResolver(config.Resolver),
		containerd.WithImageHandler(h),
		containerd.WithSchema1Conversion,
		containerd.WithPlatformMatcher(platformMC),
		containerd.WithImageHandlerWrapper(appendInfoHandlerWrapper(ref)),
	}
	opts = append(opts, config.RemoteOpts...)

	var (
		img containerd.Image
		err error
	)
	if len(config.Platforms) == 1 {
		// client.Pull is for single-platform (w/ unpacking)
		img, err = client.Pull(pctx, ref, opts...)
	} else {
		// client.Fetch is for multi-platform (w/o unpacking)
		var imagesImg images.Image
		imagesImg, err = client.Fetch(pctx, ref, opts...)
		img = containerd.NewImageWithPlatform(client, imagesImg, platformMC)
	}
	stopProgress()
	if err != nil {
		return nil, err
	}

	<-progress
	return img, nil
}

const (
	// targetRefLabel is a label which contains image reference and will be passed
	// to snapshotters.
	targetRefLabel = "containerd.io/snapshot/cri.image-ref"
	// targetManifestDigestLabel is a label which contains manifest digest and will be passed
	// to snapshotters.
	targetManifestDigestLabel = "containerd.io/snapshot/cri.manifest-digest"
	// targetLayerDigestLabel is a label which contains layer digest and will be passed
	// to snapshotters.
	targetLayerDigestLabel = "containerd.io/snapshot/cri.layer-digest"
	// targetImageLayersLabel is a label which contains layer digests contained in
	// the target image and will be passed to snapshotters for preparing layers in
	// parallel. Skipping some layers is allowed and only affects performance.
	targetImageLayersLabel = "containerd.io/snapshot/cri.image-layers"
)

func appendInfoHandlerWrapper(ref string) func(f images.Handler) images.Handler {
	return func(f images.Handler) images.Handler {
		return images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			children, err := f.Handle(ctx, desc)
			if err != nil {
				return nil, err
			}
			switch desc.MediaType {
			case ocispec.MediaTypeImageManifest, images.MediaTypeDockerSchema2Manifest:
				for i := range children {
					c := &children[i]
					if images.IsLayerType(c.MediaType) {
						if c.Annotations == nil {
							c.Annotations = make(map[string]string)
						}
						c.Annotations[targetRefLabel] = ref
						c.Annotations[targetLayerDigestLabel] = c.Digest.String()
						c.Annotations[targetImageLayersLabel] = getLayers(ctx, targetImageLayersLabel, children[i:], labels.Validate)
						c.Annotations[targetManifestDigestLabel] = desc.Digest.String()
					}
				}
			}
			return children, nil
		})
	}
}

func getLayers(ctx context.Context, key string, descs []ocispec.Descriptor, validate func(k, v string) error) (layers string) {
	var item string
	for _, l := range descs {
		if images.IsLayerType(l.MediaType) {
			item = l.Digest.String()
			if layers != "" {
				item = "," + item
			}
			// This avoids the label hits the size limitation.
			if err := validate(key, layers+item); err != nil {
				log.G(ctx).WithError(err).WithField("label", key).Debugf("%q is omitted in the layers list", l.Digest.String())
				break
			}
			layers += item
		}
	}
	return
}