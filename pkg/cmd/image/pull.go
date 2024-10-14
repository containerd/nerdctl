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

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/ipfs"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/signutil"
)

// Pull pulls an image specified by `rawRef`.
func Pull(ctx context.Context, client *containerd.Client, rawRef string, options types.ImagePullOptions) error {
	_, err := EnsureImage(ctx, client, rawRef, options)
	if err != nil {
		return err
	}

	return nil
}

// EnsureImage pulls an image either from ipfs or from registry.
func EnsureImage(ctx context.Context, client *containerd.Client, rawRef string, options types.ImagePullOptions) (*imgutil.EnsuredImage, error) {
	var ensured *imgutil.EnsuredImage

	parsedReference, err := referenceutil.Parse(rawRef)
	if err != nil {
		return nil, err
	}

	if parsedReference.Protocol != "" {
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

		ensured, err = ipfs.EnsureImage(ctx, client, string(parsedReference.Protocol), parsedReference.String(), ipfsPath, options)
		if err != nil {
			return nil, err
		}
		return ensured, nil
	}

	ref, err := signutil.Verify(ctx, rawRef, options.GOptions.HostsDir, options.GOptions.Experimental, options.VerifyOptions)
	if err != nil {
		return nil, err
	}

	ensured, err = imgutil.EnsureImage(ctx, client, ref, options)
	if err != nil {
		return nil, err
	}
	return ensured, err
}
