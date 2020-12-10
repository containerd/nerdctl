/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"fmt"
	"io"
	"strings"

	"github.com/AkihiroSuda/nerdctl/pkg/contentutil"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/stargz-snapshotter/fs/source"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type EnsuredImage struct {
	Ref         string
	Image       containerd.Image
	Snapshotter string
	Remote      bool // true for stargz
}

// PullMode is either one of "always", "missing", "never"
type PullMode = string

func EnsureImage(ctx context.Context, client *containerd.Client, stdout io.Writer, snapshotter, rawRef string, mode PullMode) (*EnsuredImage, error) {
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}
	ref := named.String()

	if mode != "always" {
		if i, err := client.ImageService().Get(ctx, ref); err == nil {
			image := containerd.NewImage(client, i)
			res := &EnsuredImage{
				Ref:         ref,
				Image:       image,
				Snapshotter: snapshotter,
				Remote:      isStargz(snapshotter),
			}
			if unpacked, err := image.IsUnpacked(ctx, snapshotter); err == nil && !unpacked {
				if err := image.Unpack(ctx, snapshotter); err != nil {
					return nil, err
				}
			}
			return res, nil
		}
	}

	if mode == "never" {
		return nil, errors.Errorf("image %q is not available", rawRef)
	}

	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(ctx)

	resovlerOpts := docker.ResolverOptions{}
	resolver := docker.NewResolver(resovlerOpts)

	var containerdImage containerd.Image
	sgz := isStargz(snapshotter)
	if sgz {
		h := images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			if desc.MediaType != images.MediaTypeDockerSchema1Manifest {
				fmt.Fprintf(stdout, "fetching %v... %v\n", desc.Digest.String()[:15], desc.MediaType)
			}
			return nil, nil
		})
		// TODO: support "skip-content-verify"
		opts := []containerd.RemoteOpt{
			containerd.WithResolver(resolver),
			containerd.WithImageHandler(h),
			containerd.WithSchema1Conversion,
			containerd.WithPullUnpack,
			containerd.WithPullSnapshotter(snapshotter),
			containerd.WithImageHandlerWrapper(source.AppendDefaultLabelsHandlerWrapper(ref, 10*1024*1024)),
		}
		containerdImage, err = client.Pull(ctx, ref, opts...)
		if err != nil {
			return nil, err
		}
	} else {
		config := &contentutil.FetchConfig{
			Resolver:        resolver,
			ProgressOutput:  stdout,
			PlatformMatcher: platforms.Default(),
		}

		img, err := contentutil.Fetch(ctx, client, ref, config)
		if err != nil {
			return nil, err
		}
		containerdImage = containerd.NewImageWithPlatform(client, img, config.PlatformMatcher)
		if err := containerdImage.Unpack(ctx, snapshotter); err != nil {
			return nil, err
		}
	}
	res := &EnsuredImage{
		Ref:         ref,
		Image:       containerdImage,
		Snapshotter: snapshotter,
		Remote:      sgz,
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
