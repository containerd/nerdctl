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
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/stargz-snapshotter/ipfs"
	ipfsclient "github.com/containerd/stargz-snapshotter/ipfs/client"
	"github.com/docker/docker/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

const ipfsPathEnv = "IPFS_PATH"

// EnsureImage pull the specified image from IPFS.
func EnsureImage(ctx context.Context, client *containerd.Client, stdout, stderr io.Writer, snapshotter string, scheme string, ref string, mode imgutil.PullMode, ocispecPlatforms []ocispec.Platform, unpack *bool, quiet bool, ipfsPath string) (*imgutil.EnsuredImage, error) {
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

	// if not `always` pull and given one platform and image found locally, return existing image directly.
	if mode != "always" && len(ocispecPlatforms) == 1 {
		if res, err := imgutil.GetExistingImage(ctx, client, snapshotter, ref, ocispecPlatforms[0]); err == nil {
			return res, nil
		} else if !errdefs.IsNotFound(err) {
			return nil, err
		}
	}

	if mode == "never" {
		return nil, fmt.Errorf("image %q is not available", ref)
	}
	r, err := ipfs.NewResolver(ipfs.ResolverOptions{
		Scheme:   scheme,
		IPFSPath: lookupIPFSPath(ipfsPath),
	})
	if err != nil {
		return nil, err
	}
	return imgutil.PullImage(ctx, client, stdout, stderr, snapshotter, r, ref, ocispecPlatforms, unpack, quiet)
}

// Push pushes the specified image to IPFS.
func Push(ctx context.Context, client *containerd.Client, rawRef string, layerConvert converter.ConvertFunc, allPlatforms bool, platform []string, ensureImage bool, ipfsPath string) (string, error) {
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return "", err
	}
	ipath := lookupIPFSPath(ipfsPath)
	if ensureImage {
		// Ensure image contents are fully downloaded
		logrus.Infof("ensuring image contents")
		if err := ensureContentsOfIPFSImage(ctx, client, rawRef, allPlatforms, platform, ipath); err != nil {
			logrus.WithError(err).Warnf("failed to ensure the existence of image %q", rawRef)
		}
	}
	ref, err := referenceutil.ParseAny(rawRef)
	if err != nil {
		return "", err
	}
	return ipfs.PushWithIPFSPath(ctx, client, ref.String(), layerConvert, platMC, &ipath)
}

// ensureContentsOfIPFSImage ensures that the entire contents of an existing IPFS image are fully downloaded to containerd.
func ensureContentsOfIPFSImage(ctx context.Context, client *containerd.Client, ref string, allPlatforms bool, platform []string, ipfsPath string) error {
	iurl, err := ipfsclient.GetIPFSAPIAddress(ipfsPath, "http")
	if err != nil {
		return err
	}
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
		return fmt.Errorf("ambiguous reference %q matched %d objects", ref, n)
	}
	cs := client.ContentStore()
	childrenHandler := images.ChildrenHandler(cs)
	childrenHandler = images.SetChildrenLabels(cs, childrenHandler)
	childrenHandler = images.FilterPlatforms(childrenHandler, platMC)
	return images.Dispatch(ctx, images.Handlers(
		remotes.FetchHandler(cs, &fetcher{ipfsclient.New(iurl)}),
		childrenHandler,
	), nil, img.Target)
}

// fetcher fetches a file from IPFS
// TODO: fix github.com/containerd/stargz-snapshotter/ipfs to export this and we should import that
type fetcher struct {
	ipfsclient *ipfsclient.Client
}

func (f *fetcher) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	cid, err := ipfs.GetCID(desc)
	if err != nil {
		return nil, err
	}
	off, size := 0, int(desc.Size)
	return f.ipfsclient.Get("/ipfs/"+cid, &off, &size)
}

// If IPFS_PATH is specified, this will be used.
// If not, "~/.ipfs" will be used.
// The behaviour is compatible to kubo: https://github.com/ipfs/go-ipfs-http-client/blob/171fcd55e3b743c38fb9d78a34a3a703ee0b5e89/api.go#L43-L44
// Optionally takes ipfsPath string having the highest priority.
func lookupIPFSPath(ipfsPath string) string {
	var ipath string
	if idir := os.Getenv(ipfsPathEnv); idir != "" {
		ipath = idir
	}
	if ipath == "" {
		ipath = "~/.ipfs"
	}
	if ipfsPath != "" {
		ipath = ipfsPath
	}
	return ipath
}
