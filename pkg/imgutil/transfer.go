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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/core/transfer"
	transferimage "github.com/containerd/containerd/v2/core/transfer/image"
	"github.com/containerd/containerd/v2/core/transfer/registry"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/transferutil"
)

func prepareImageStore(ctx context.Context, parsedReference *referenceutil.ImageReference, options types.ImagePullOptions) (*transferimage.Store, error) {
	var storeOpts []transferimage.StoreOpt
	if len(options.OCISpecPlatform) > 0 {
		storeOpts = append(storeOpts, transferimage.WithPlatforms(options.OCISpecPlatform...))
	}

	unpackEnabled := len(options.OCISpecPlatform) == 1
	if options.Unpack != nil {
		unpackEnabled = *options.Unpack
		if unpackEnabled && len(options.OCISpecPlatform) != 1 {
			return nil, fmt.Errorf("unpacking requires a single platform to be specified (e.g., --platform=amd64)")
		}
	}

	if unpackEnabled {
		platform := options.OCISpecPlatform[0]
		snapshotter := options.GOptions.Snapshotter
		storeOpts = append(storeOpts, transferimage.WithUnpack(platform, snapshotter))
	}

	return transferimage.NewStore(parsedReference.String(), storeOpts...), nil
}

func createOCIRegistry(ctx context.Context, parsedReference *referenceutil.ImageReference, gOptions types.GlobalCommandOptions, plainHTTP bool) (*registry.OCIRegistry, func(), error) {
	ch, err := dockerconfigresolver.NewCredentialHelper(parsedReference.Domain)
	if err != nil {
		return nil, nil, err
	}

	opts := []registry.Opt{
		registry.WithCredentials(ch),
	}

	var tmpHostsDir string
	cleanup := func() {
		if tmpHostsDir != "" {
			os.RemoveAll(tmpHostsDir)
		}
	}

	// If insecure-registry is set, create a temporary hosts.toml with skip_verify
	if gOptions.InsecureRegistry {
		tmpHostsDir, err = dockerconfigresolver.CreateTmpHostsConfig(parsedReference.Domain, true)
		if err != nil {
			log.G(ctx).WithError(err).Warnf("failed to create temporary hosts.toml for %q, continuing without it", parsedReference.Domain)
		} else if tmpHostsDir != "" {
			opts = append(opts, registry.WithHostDir(tmpHostsDir))
		}
	} else if len(gOptions.HostsDir) > 0 {
		opts = append(opts, registry.WithHostDir(gOptions.HostsDir[0]))
	}

	if isLocalHost, err := docker.MatchLocalhost(parsedReference.Domain); err != nil {
		cleanup()
		return nil, nil, err
	} else if isLocalHost || plainHTTP {
		opts = append(opts, registry.WithDefaultScheme("http"))
	}

	reg, err := registry.NewOCIRegistry(ctx, parsedReference.String(), opts...)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	return reg, cleanup, nil
}

func PullImageWithTransfer(ctx context.Context, client *containerd.Client, parsedReference *referenceutil.ImageReference, rawRef string, options types.ImagePullOptions) (*EnsuredImage, error) {
	store, err := prepareImageStore(ctx, parsedReference, options)
	if err != nil {
		return nil, err
	}

	progressWriter := options.Stderr
	if options.ProgressOutputToStdout {
		progressWriter = options.Stdout
	}

	fetcher, cleanup, err := createOCIRegistry(ctx, parsedReference, options.GOptions, false)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	transferErr := doTransfer(ctx, client, fetcher, store, options.Quiet, progressWriter)

	if transferErr != nil && (errors.Is(transferErr, http.ErrSchemeMismatch) || errutil.IsErrConnectionRefused(transferErr) || errutil.IsErrHTTPResponseToHTTPSClient(transferErr) || errutil.IsErrTLSHandshakeFailure(transferErr)) {
		if options.GOptions.InsecureRegistry {
			log.G(ctx).WithError(transferErr).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", parsedReference.Domain)
			fetcher, cleanup2, err := createOCIRegistry(ctx, parsedReference, options.GOptions, true)
			if err != nil {
				return nil, err
			}
			defer cleanup2()
			transferErr = doTransfer(ctx, client, fetcher, store, options.Quiet, progressWriter)
		}
	}

	if transferErr != nil {
		return nil, transferErr
	}

	imageStore := client.ImageService()
	stored, err := store.Get(ctx, imageStore)
	if err != nil {
		return nil, err
	}

	plMatch := platformutil.NewMatchComparerFromOCISpecPlatformSlice(options.OCISpecPlatform)
	containerdImage := containerd.NewImageWithPlatform(client, stored, plMatch)
	imgConfig, err := getImageConfig(ctx, containerdImage)
	if err != nil {
		return nil, err
	}

	snapshotter := options.GOptions.Snapshotter
	snOpt := getSnapshotterOpts(snapshotter)

	return &EnsuredImage{
		Ref:         rawRef,
		Image:       containerdImage,
		ImageConfig: *imgConfig,
		Snapshotter: snapshotter,
		Remote:      snOpt.isRemote(),
	}, nil
}

func preparePushStore(pushRef string, options types.ImagePushOptions) (*transferimage.Store, error) {
	platformsSlice, err := platformutil.NewOCISpecPlatformSlice(options.AllPlatforms, options.Platforms)
	if err != nil {
		return nil, err
	}

	storeOpts := []transferimage.StoreOpt{}
	if len(platformsSlice) > 0 {
		storeOpts = append(storeOpts, transferimage.WithPlatforms(platformsSlice...))
	}

	return transferimage.NewStore(pushRef, storeOpts...), nil
}

func PushImageWithTransfer(ctx context.Context, client *containerd.Client, parsedReference *referenceutil.ImageReference, pushRef, rawRef string, options types.ImagePushOptions) error {
	source, err := preparePushStore(pushRef, options)
	if err != nil {
		return err
	}

	progressWriter := io.Discard
	if options.Stdout != nil {
		progressWriter = options.Stdout
	}

	pusher, cleanup, err := createOCIRegistry(ctx, parsedReference, options.GOptions, false)
	if err != nil {
		return err
	}
	defer cleanup()

	transferErr := doTransfer(ctx, client, source, pusher, options.Quiet, progressWriter)

	if transferErr != nil && (errors.Is(transferErr, http.ErrSchemeMismatch) || errutil.IsErrConnectionRefused(transferErr) || errutil.IsErrHTTPResponseToHTTPSClient(transferErr) || errutil.IsErrTLSHandshakeFailure(transferErr)) {
		if options.GOptions.InsecureRegistry {
			log.G(ctx).WithError(transferErr).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", parsedReference.Domain)
			pusher, cleanup2, err := createOCIRegistry(ctx, parsedReference, options.GOptions, true)
			if err != nil {
				return err
			}
			defer cleanup2()
			transferErr = doTransfer(ctx, client, source, pusher, options.Quiet, progressWriter)
		}
	}

	if transferErr != nil {
		log.G(ctx).WithError(transferErr).Errorf("server %q does not seem to support HTTPS", parsedReference.Domain)
		log.G(ctx).Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
		return transferErr
	}

	return nil
}

func doTransfer(ctx context.Context, client *containerd.Client, src, dst interface{}, quiet bool, progressWriter io.Writer) error {
	opts := make([]transfer.Opt, 0, 1)
	if !quiet {
		pf, done := transferutil.ProgressHandler(ctx, progressWriter)
		defer done()
		opts = append(opts, transfer.WithProgress(pf))
	}
	return client.Transfer(ctx, src, dst, opts...)
}
