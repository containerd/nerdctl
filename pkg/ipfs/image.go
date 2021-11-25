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

package ipfs

import (
	"context"
	"fmt"
	"io"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/stargz-snapshotter/ipfs"
	"github.com/docker/docker/errdefs"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

// EnsureImage pull the specified image from IPFS.
func EnsureImage(ctx context.Context, client *containerd.Client, ipfsClient iface.CoreAPI, stdout, stderr io.Writer, snapshotter string, scheme string, ref string, mode imgutil.PullMode, ocispecPlatforms []ocispec.Platform, unpack *bool) (*imgutil.EnsuredImage, error) {
	switch mode {
	case "always", "missing", "never":
		// NOP
	default:
		return nil, fmt.Errorf("unexpected pull mode: %q", mode)
	}
	switch scheme {
	case "ipfs", "ipns":
		// NOP
	default:
		return nil, fmt.Errorf("unexpected scheme: %q", scheme)
	}

	if mode != "always" && len(ocispecPlatforms) == 1 {
		res, err := imgutil.GetExistingImage(ctx, client, snapshotter, ref, ocispecPlatforms[0])
		if err == nil {
			return res, nil
		}
		if !errdefs.IsNotFound(err) {
			return nil, err
		}
	}

	if mode == "never" {
		return nil, fmt.Errorf("image %q is not available", ref)
	}
	r, err := ipfs.NewResolver(ipfsClient, ipfs.ResolverOptions{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	return imgutil.PullImage(ctx, client, stdout, stderr, snapshotter, r, ref, ocispecPlatforms, unpack)
}

// Push pushes the specified image to IPFS.
func Push(ctx context.Context, client *containerd.Client, ipfsClient iface.CoreAPI, rawRef string, layerConvert converter.ConvertFunc, allPlatforms bool, platform []string, ensureImage bool) (cid.Cid, error) {
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return cid.Cid{}, err
	}
	if ensureImage {
		// Ensure image contents are fully downloaded
		logrus.Infof("ensuring image contents")
		if err := ensureContentsOfIPFSImage(ctx, client, ipfsClient, rawRef, allPlatforms, platform); err != nil {
			logrus.WithError(err).Warnf("failed to ensure the existence of image %q", rawRef)
		}
	}
	ref, err := referenceutil.ParseAny(rawRef)
	if err != nil {
		return cid.Cid{}, err
	}
	p, err := ipfs.Push(ctx, client, ipfsClient, ref.String(), layerConvert, platMC)
	if err != nil {
		return cid.Cid{}, err
	}
	return p.Cid(), nil
}

// ensureContentsOfIPFSImage ensures that the entire contents of an exisiting IPFS image are fully downloaded to containerd.
func ensureContentsOfIPFSImage(ctx context.Context, client *containerd.Client, ipfsClient iface.CoreAPI, ref string, allPlatforms bool, platform []string) error {
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return err
	}
	var img images.Image
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			img = found.Image
			return nil
		},
	}
	n, err := walker.Walk(ctx, ref)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("image does not exist: %q", ref)
	} else if n > 1 {
		return fmt.Errorf("ambigious reference %q matched %d objects", ref, n)
	}
	cs := client.ContentStore()
	childrenHandler := images.ChildrenHandler(cs)
	childrenHandler = images.SetChildrenLabels(cs, childrenHandler)
	childrenHandler = images.FilterPlatforms(childrenHandler, platMC)
	return images.Dispatch(ctx, images.Handlers(
		remotes.FetchHandler(cs, &fetcher{ipfsClient}),
		childrenHandler,
	), nil, img.Target)
}

// fetcher fetches a file from IPFS
// TODO: fix github.com/containerd/stargz-snapshotter/ipfs to export this and we should import that
type fetcher struct {
	api iface.CoreAPI
}

func (f *fetcher) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	p, err := ipfs.GetPath(desc)
	if err != nil {
		return nil, err
	}
	n, err := f.api.Unixfs().Get(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("failed to get file %q: %v", p.String(), err)
	}
	return files.ToFile(n), nil
}
