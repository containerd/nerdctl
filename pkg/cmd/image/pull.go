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
	"os"
	"path/filepath"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/ipfs"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/nerdctl/pkg/signutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Pull pulls an image specified by `rawRef`.
func Pull(ctx context.Context, client *containerd.Client, rawRef string, options types.ImagePullOptions) error {
	ocispecPlatforms, err := platformutil.NewOCISpecPlatformSlice(options.AllPlatforms, options.Platform)
	if err != nil {
		return err
	}

	unpack, err := strutil.ParseBoolOrAuto(options.Unpack)
	if err != nil {
		return err
	}

	_, err = EnsureImage(ctx, client, rawRef, ocispecPlatforms, "always", unpack, options.Quiet, options)
	if err != nil {
		return err
	}

	return nil
}

// EnsureImage pulls an image either from ipfs or from registry.
func EnsureImage(ctx context.Context, client *containerd.Client, rawRef string, ocispecPlatforms []v1.Platform, pull string, unpack *bool, quiet bool, options types.ImagePullOptions) (*imgutil.EnsuredImage, error) {
	var ensured *imgutil.EnsuredImage

	if scheme, ref, err := referenceutil.ParseIPFSRefWithScheme(rawRef); err == nil {
		if options.VerifyOptions.Provider != "none" {
			return nil, errors.New("--verify flag is not supported on IPFS as of now")
		}

		var ipfsPath string
		if options.IPFSAddress != "" {
			dir, err := os.MkdirTemp("", "apidirtmp")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(dir)
			if err := os.WriteFile(filepath.Join(dir, "api"), []byte(options.IPFSAddress), 0600); err != nil {
				return nil, err
			}
			ipfsPath = dir
		}

		ensured, err = ipfs.EnsureImage(ctx, client, options.Stdout, options.Stderr, options.GOptions.Snapshotter, scheme, ref,
			pull, ocispecPlatforms, unpack, quiet, ipfsPath)
		if err != nil {
			return nil, err
		}
		return ensured, nil
	}

	ref, err := signutil.Verify(ctx, rawRef, options.GOptions.HostsDir, options.GOptions.Experimental, options.VerifyOptions)
	if err != nil {
		return nil, err
	}

	ensured, err = imgutil.EnsureImage(ctx, client, options.Stdout, options.Stderr, options.GOptions.Snapshotter, ref,
		pull, options.GOptions.InsecureRegistry, options.GOptions.HostsDir, ocispecPlatforms, unpack, quiet)
	if err != nil {
		return nil, err
	}
	return ensured, err
}
