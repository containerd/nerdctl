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

package compose

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/volume"
	"github.com/containerd/nerdctl/v2/pkg/composer"
	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/ipfs"
	"github.com/containerd/nerdctl/v2/pkg/lockutil"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/signutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

//nolint:unused
var locked *os.File

// New returns a new *composer.Composer.
func New(client *containerd.Client, globalOptions types.GlobalCommandOptions, options composer.Options, stdout, stderr io.Writer) (*composer.Composer, error) {
	// Compose right now cannot be made safe to use concurrently, as we shell out to nerdctl for multiple operations,
	// preventing us from using the lock mechanisms from the API.
	// This here imposes a global lock, effectively preventing multiple compose commands from being run in parallel and
	// preventing some of the problems with concurrent execution.
	// This should be removed once we have better, in-depth solutions to make this concurrency safe.
	// Note that we do not close the lock explicitly. Instead, the lock will get released when the `locked` global
	// variable will get collected and the file descriptor closed (eg: when the binary exits).
	var err error
	locked, err = lockutil.Lock(globalOptions.DataRoot)
	if err != nil {
		return nil, err
	}

	cniEnv, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath, netutil.WithNamespace(globalOptions.Namespace), netutil.WithDefaultNetwork())
	if err != nil {
		return nil, err
	}
	networkConfigs, err := cniEnv.NetworkList()
	if err != nil {
		return nil, err
	}
	options.NetworkExists = func(netName string) (bool, error) {
		for _, f := range networkConfigs {
			if f.Name == netName {
				return true, nil
			}
		}
		return false, nil
	}

	options.NetworkInUse = func(ctx context.Context, netName string) (bool, error) {
		networkUsedByNsMap, err := netutil.UsedNetworks(ctx, client)
		if err != nil {
			return false, err
		}
		for _, v := range networkUsedByNsMap {
			if strutil.InStringSlice(v, netName) {
				return true, nil
			}
		}
		return false, nil
	}

	volStore, err := volume.Store(globalOptions.Namespace, globalOptions.DataRoot, globalOptions.Address)
	if err != nil {
		return nil, err
	}
	// FIXME: this is racy. See note in up_volume.go
	options.VolumeExists = volStore.Exists

	options.ImageExists = func(ctx context.Context, rawRef string) (bool, error) {
		parsedReference, err := referenceutil.Parse(rawRef)
		if err != nil {
			return false, err
		}
		ref := parsedReference.String()
		if _, err := client.ImageService().Get(ctx, ref); err != nil {
			if errors.Is(err, errdefs.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	options.EnsureImage = func(ctx context.Context, imageName, pullMode, platform string, ps *serviceparser.Service, quiet bool) error {
		ocispecPlatforms := []ocispec.Platform{platforms.DefaultSpec()}
		if platform != "" {
			parsed, err := platforms.Parse(platform)
			if err != nil {
				return err
			}
			ocispecPlatforms = []ocispec.Platform{parsed} // no append
		}

		imgPullOpts := types.ImagePullOptions{
			GOptions:        globalOptions,
			OCISpecPlatform: ocispecPlatforms,
			Unpack:          nil,
			Mode:            pullMode,
			Quiet:           quiet,
			RFlags:          types.RemoteSnapshotterFlags{},
			Stdout:          stdout,
			Stderr:          stderr,
		}

		parsedReference, err := referenceutil.Parse(imageName)
		if err != nil {
			return err
		}

		if parsedReference.Protocol != "" {
			var ipfsPath string
			if ipfsAddress := options.IPFSAddress; ipfsAddress != "" {
				dir, err := os.MkdirTemp("", "apidirtmp")
				if err != nil {
					return err
				}
				defer os.RemoveAll(dir)
				if err := os.WriteFile(filepath.Join(dir, "api"), []byte(ipfsAddress), 0600); err != nil {
					return err
				}
				ipfsPath = dir
			}
			_, err = ipfs.EnsureImage(ctx, client, string(parsedReference.Protocol), parsedReference.String(), ipfsPath, imgPullOpts)
			return err
		}

		imageVerifyOptions := imageVerifyOptionsFromCompose(ps)
		ref, err := signutil.Verify(ctx, imageName, globalOptions.HostsDir, globalOptions.Experimental, imageVerifyOptions)
		if err != nil {
			return err
		}

		_, err = imgutil.EnsureImage(ctx, client, ref, imgPullOpts)
		return err
	}

	return composer.New(options, client)
}

func imageVerifyOptionsFromCompose(ps *serviceparser.Service) types.ImageVerifyOptions {
	var opt types.ImageVerifyOptions
	if verifier, ok := ps.Unparsed.Extensions[serviceparser.ComposeVerify]; ok {
		opt.Provider = verifier.(string)
	} else {
		opt.Provider = "none"
	}

	// for cosign, if key is given, use key mode, otherwise use keyless mode.
	if keyVal, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignPublicKey]; ok {
		opt.CosignKey = keyVal.(string)
	}
	if ciVal, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateIdentity]; ok {
		opt.CosignCertificateIdentity = ciVal.(string)
	}
	if cirVal, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateIdentityRegexp]; ok {
		opt.CosignCertificateIdentityRegexp = cirVal.(string)
	}
	if coiVal, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateOidcIssuer]; ok {
		opt.CosignCertificateOidcIssuer = coiVal.(string)
	}
	if coirVal, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateOidcIssuerRegexp]; ok {
		opt.CosignCertificateOidcIssuerRegexp = coirVal.(string)
	}
	return opt
}
