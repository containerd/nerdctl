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
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/volume"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/ipfs"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/nerdctl/pkg/signutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

func New(client *containerd.Client, globalOptions types.GlobalCommandOptions, options composer.Options, stdout, stderr io.Writer) (*composer.Composer, error) {
	cniEnv, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath, netutil.WithDefaultNetwork())
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
	options.VolumeExists = func(volName string) (bool, error) {
		if _, volGetErr := volStore.Get(volName, false); volGetErr == nil {
			return true, nil
		} else if errors.Is(volGetErr, errdefs.ErrNotFound) {
			return false, nil
		} else {
			return false, volGetErr
		}
	}

	options.ImageExists = func(ctx context.Context, rawRef string) (bool, error) {
		refNamed, err := referenceutil.ParseAny(rawRef)
		if err != nil {
			return false, err
		}
		ref := refNamed.String()
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

		// IPFS reference
		if scheme, ref, err := referenceutil.ParseIPFSRefWithScheme(imageName); err == nil {
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
			_, err = ipfs.EnsureImage(ctx, client, stdout, stderr, globalOptions.Snapshotter, scheme, ref,
				pullMode, ocispecPlatforms, nil, quiet, ipfsPath)
			return err
		}

		ref := imageName
		if verifier, ok := ps.Unparsed.Extensions[serviceparser.ComposeVerify]; ok {
			switch verifier {
			case "cosign":
				if !options.Experimental {
					return fmt.Errorf("cosign only work with enable experimental feature")
				}

				// if key is given, use key mode, otherwise use keyless mode.
				keyRef := ""
				if keyVal, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignPublicKey]; ok {
					keyRef = keyVal.(string)
				}
				ref, err = signutil.VerifyCosign(ctx, ref, keyRef, globalOptions.HostsDir)
				if err != nil {
					return err
				}
			case "none":
				logrus.Debugf("verification process skipped")
			default:
				return fmt.Errorf("no verifier found: %s", verifier)
			}
		}
		_, err := imgutil.EnsureImage(ctx, client, stdout, stderr, globalOptions.Snapshotter, ref,
			pullMode, globalOptions.InsecureRegistry, globalOptions.HostsDir, ocispecPlatforms, nil, quiet)
		return err
	}

	return composer.New(options, client)
}
