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
	"net/http"
	"os"

	distributionref "github.com/distribution/reference"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/fetch"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
)

func EnsureAllContent(ctx context.Context, client *containerd.Client, srcName string, options types.GlobalCommandOptions) error {
	// Get the image from the srcName
	imageService := client.ImageService()
	img, err := imageService.Get(ctx, srcName)
	if err != nil {
		return err
	}

	provider := containerdutil.NewProvider(client)
	snapshotter := containerdutil.SnapshotService(client, options.Snapshotter)
	// Read the image
	imagesList, _ := read(ctx, provider, snapshotter, img.Target)
	// Iterate through the list
	for _, i := range imagesList {
		err = ensureOne(ctx, client, srcName, img.Target, i.platform, options)
		if err != nil {
			return err
		}
	}

	return nil
}

func ensureOne(ctx context.Context, client *containerd.Client, rawRef string, target ocispec.Descriptor, platform ocispec.Platform, options types.GlobalCommandOptions) error {

	named, err := distributionref.ParseDockerRef(rawRef)
	if err != nil {
		return err
	}
	refDomain := distributionref.Domain(named)
	// if platform == nil {
	//	platform = platforms.DefaultSpec()
	//}
	pltf := []ocispec.Platform{platform}
	platformComparer := platformutil.NewMatchComparerFromOCISpecPlatformSlice(pltf)

	_, _, _, missing, err := images.Check(ctx, client.ContentStore(), target, platformComparer)
	if err != nil {
		return err
	}

	if len(missing) > 0 {
		// Get a resolver
		var dOpts []dockerconfigresolver.Opt
		if options.InsecureRegistry {
			log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", refDomain)
			dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
		}
		dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(options.HostsDir))
		resolver, err := dockerconfigresolver.New(ctx, refDomain, dOpts...)
		if err != nil {
			return err
		}
		config := &fetch.Config{
			Resolver:       resolver,
			RemoteOpts:     []containerd.RemoteOpt{},
			Platforms:      pltf,
			ProgressOutput: os.Stderr,
		}

		err = fetch.Fetch(ctx, client, rawRef, config)

		if err != nil {
			// In some circumstance (e.g. people just use 80 port to support pure http), the error will contain message like "dial tcp <port>: connection refused".
			if !errors.Is(err, http.ErrSchemeMismatch) && !errutil.IsErrConnectionRefused(err) {
				return err
			}
			if options.InsecureRegistry {
				log.G(ctx).WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", refDomain)
				dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
				resolver, err = dockerconfigresolver.New(ctx, refDomain, dOpts...)
				if err != nil {
					return err
				}
				config.Resolver = resolver
				return fetch.Fetch(ctx, client, rawRef, config)
			}
			log.G(ctx).WithError(err).Errorf("server %q does not seem to support HTTPS", refDomain)
			log.G(ctx).Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
		}

		return err
	}

	return nil
}
