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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	dockerconfig "github.com/containerd/containerd/v2/core/remotes/docker/config"
	"github.com/containerd/containerd/v2/pkg/reference"
	"github.com/containerd/log"
	"github.com/containerd/platforms"
	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	nerdconverter "github.com/containerd/nerdctl/v2/pkg/imgutil/converter"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/push"
	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
	"github.com/containerd/nerdctl/v2/pkg/ipfs"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/signutil"
	"github.com/containerd/nerdctl/v2/pkg/snapshotterutil"
)

const (
	// Suffixes of the temporary images push creates for itself before uploading them.
	tmpReducedPlatformSuffix = "-tmp-reduced-platform"
	tmpEsgzSuffix            = "-tmp-esgz"
)

// Push pushes an image specified by `rawRef`.
// With options.AllTags, `rawRef` must be a bare repository name, and every local tag of that
// repository is pushed.
func Push(ctx context.Context, client *containerd.Client, rawRef string, options types.ImagePushOptions) error {
	if !options.AllTags {
		return pushSingle(ctx, client, rawRef, options, false)
	}

	parsedReference, err := referenceutil.Parse(rawRef)
	if err != nil {
		return err
	}
	// ExplicitTag, not Tag: Parse normalizes a bare repository name to ":latest".
	if parsedReference.ExplicitTag != "" || parsedReference.Digest != "" {
		return errors.New("tag can't be used with --all-tags/-a")
	}
	if parsedReference.Protocol != "" {
		return fmt.Errorf("--all-tags is not supported for %q references", parsedReference.Protocol)
	}

	imgs, err := localTags(ctx, client, parsedReference.Name())
	if err != nil {
		return err
	}
	if len(imgs) == 0 {
		return fmt.Errorf("an image does not exist locally with the tag: %s", parsedReference.Name())
	}

	// A SOCI index is attached to the image manifest rather than to the tag, so it only needs to be
	// built once per distinct target. Doing it per tag makes every tag overwrite the index pushed by
	// the previous one: https://github.com/containerd/nerdctl/issues/3751
	indexed := make(map[digest.Digest]struct{}, len(imgs))
	for _, img := range imgs {
		_, done := indexed[img.Target.Digest]
		if err = pushSingle(ctx, client, img.Name, options, done); err != nil {
			return err
		}
		indexed[img.Target.Digest] = struct{}{}
	}
	return nil
}

// localTags returns the local images tagged under the repository `name`, sorted by name.
func localTags(ctx context.Context, client *containerd.Client, name string) ([]images.Image, error) {
	// Same idiom as nameFilterFor() in cmd/nerdctl/image/image_list.go: a bare repository name
	// matches every tag of that repository (pkg/ cannot import cmd/, hence the repeated filter).
	imgs, err := client.ImageService().List(ctx, fmt.Sprintf("name~=^%s:", regexp.QuoteMeta(name)))
	if err != nil {
		return nil, err
	}
	// Drop the temporary images push creates for itself, which an interrupted push may have left behind.
	imgs = slices.DeleteFunc(imgs, func(img images.Image) bool {
		return strings.HasSuffix(img.Name, tmpReducedPlatformSuffix) || strings.HasSuffix(img.Name, tmpEsgzSuffix)
	})
	// ImageService().List does not guarantee an order, and the order decides which tag gets indexed.
	slices.SortFunc(imgs, func(a, b images.Image) int {
		return strings.Compare(a.Name, b.Name)
	})
	return imgs, nil
}

func pushSingle(ctx context.Context, client *containerd.Client, rawRef string, options types.ImagePushOptions, skipSoci bool) error {
	parsedReference, err := referenceutil.Parse(rawRef)
	if err != nil {
		return err
	}

	if parsedReference.Protocol != "" {
		if parsedReference.Protocol != referenceutil.IPFSProtocol {
			return fmt.Errorf("ipfs scheme is only supported but got %q", parsedReference.Protocol)
		}
		log.G(ctx).Infof("pushing image %q to IPFS", parsedReference)

		// Ensure all the layers are here: https://github.com/containerd/nerdctl/issues/3489
		// XXX what if the image is a CID, or only otherwise available on ipfs?
		platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platforms)
		if err != nil {
			return err
		}

		err = EnsureAllContent(ctx, client, parsedReference.String(), platMC, options.GOptions)
		if err != nil {
			return err
		}

		var ipfsPath string
		if options.IpfsAddress != "" {
			dir, err := os.MkdirTemp("", "apidirtmp")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			if err := filesystem.WriteFile(filepath.Join(dir, "api"), []byte(options.IpfsAddress), 0600); err != nil {
				return err
			}
			ipfsPath = dir
		}

		var layerConvert converter.ConvertFunc
		if options.Estargz {
			layerConvert = eStargzConvertFunc()
		}
		c, err := ipfs.Push(ctx, client, parsedReference.String(), layerConvert, options.AllPlatforms, options.Platforms, options.IpfsEnsureImage, ipfsPath)
		if err != nil {
			log.G(ctx).WithError(err).Warnf("ipfs push failed")
			return err
		}
		fmt.Fprintln(options.Stdout, c)
		return nil
	}

	parsedReference, err = referenceutil.Parse(rawRef)
	if err != nil {
		return err
	}
	ref := parsedReference.String()

	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platforms)
	if err != nil {
		return err
	}
	pushRef := ref
	if !options.AllPlatforms {
		pushRef = ref + tmpReducedPlatformSuffix
		// Push fails with "400 Bad Request" when the manifest is multi-platform but we do not locally have multi-platform blobs.
		// So we create a tmp reduced-platform image to avoid the error.
		// Ensure all the layers are here: https://github.com/containerd/nerdctl/issues/3425
		err = EnsureAllContent(ctx, client, ref, platMC, options.GOptions)
		if err != nil {
			return err
		}
		platImg, err := nerdconverter.Convert(ctx, client, pushRef, ref, converter.WithPlatform(platMC))
		if err != nil {
			if len(options.Platforms) == 0 {
				return fmt.Errorf("failed to create a tmp single-platform image %q: %w", pushRef, err)
			}
			return fmt.Errorf("failed to create a tmp reduced-platform image %q (platform=%v): %w", pushRef, options.Platforms, err)
		}
		defer client.ImageService().Delete(ctx, platImg.Name, images.SynchronousDelete())
		log.G(ctx).Infof("pushing as a reduced-platform image (%s, %s)", platImg.Target.MediaType, platImg.Target.Digest)
	}

	if options.Estargz {
		pushRef = ref + tmpEsgzSuffix
		esgzImg, err := nerdconverter.Convert(ctx, client, pushRef, ref, converter.WithPlatform(platMC), converter.WithLayerConvertFunc(eStargzConvertFunc()))
		if err != nil {
			return fmt.Errorf("failed to convert to eStargz: %v", err)
		}
		defer client.ImageService().Delete(ctx, esgzImg.Name, images.SynchronousDelete())
		log.G(ctx).Infof("pushing as an eStargz image (%s, %s)", esgzImg.Target.MediaType, esgzImg.Target.Digest)
	}
	if !options.AllowNondistributableArtifacts {
		if err := pushImageWithLocal(ctx, client, parsedReference, pushRef, ref, options, platMC); err != nil {
			return err
		}
	} else {
		// Transfer service is available in containerd 1.7, but full support is only in 2.0+
		// For containerd 1.7, use the legacy resolver-based push method for better compatibility
		useTransferAPI := containerdutil.SupportsFullTransferService(ctx, client)
		if !useTransferAPI {
			log.G(ctx).Debug("Detected containerd < 2.0, using legacy push method")
		}

		if useTransferAPI {
			if err := imgutil.PushImageWithTransfer(ctx, client, parsedReference, pushRef, ref, options); err != nil {
				return err
			}
		} else {
			if err := pushImageWithLocal(ctx, client, parsedReference, pushRef, ref, options, platMC); err != nil {
				return err
			}
		}
	}

	img, err := client.ImageService().Get(ctx, pushRef)
	if err != nil {
		return err
	}
	refSpec, err := reference.Parse(pushRef)
	if err != nil {
		return err
	}
	signRef := fmt.Sprintf("%s@%s", refSpec.String(), img.Target.Digest.String())
	if err = signutil.Sign(signRef,
		options.GOptions.Experimental,
		options.SignOptions); err != nil {
		return err
	}
	if options.GOptions.Snapshotter == "soci" && !skipSoci {
		if err = snapshotterutil.CreateSociIndexV1(ref, options.GOptions, options.AllPlatforms, options.Platforms, options.SociOptions); err != nil {
			return err
		}
		if err = snapshotterutil.PushSoci(ref, options.GOptions, options.AllPlatforms, options.Platforms); err != nil {
			return err
		}
	}
	if options.Quiet {
		fmt.Fprintln(options.Stdout, ref)
	}
	return nil
}

func eStargzConvertFunc() converter.ConvertFunc {
	convertToESGZ := estargzconvert.LayerConvertFunc()
	return func(ctx context.Context, cs content.Store, desc ocispec.Descriptor) (*ocispec.Descriptor, error) {
		if isReusableESGZ(ctx, cs, desc) {
			log.L.Infof("reusing estargz %s without conversion", desc.Digest)
			return nil, nil
		}
		newDesc, err := convertToESGZ(ctx, cs, desc)
		if err != nil {
			return nil, err
		}
		log.L.Infof("converted %q to %s", desc.MediaType, newDesc.Digest)
		return newDesc, err
	}

}

func isReusableESGZ(ctx context.Context, cs content.Store, desc ocispec.Descriptor) bool {
	dgstStr, ok := desc.Annotations[estargz.TOCJSONDigestAnnotation]
	if !ok {
		return false
	}
	tocdgst, err := digest.Parse(dgstStr)
	if err != nil {
		return false
	}
	ra, err := cs.ReaderAt(ctx, desc)
	if err != nil {
		return false
	}
	defer ra.Close()
	r, err := estargz.Open(io.NewSectionReader(ra, 0, desc.Size), estargz.WithDecompressors(new(zstdchunked.Decompressor)))
	if err != nil {
		return false
	}
	if _, err := r.VerifyTOC(tocdgst); err != nil {
		return false
	}
	return true
}

func pushImageWithLocal(ctx context.Context, client *containerd.Client, parsedReference *referenceutil.ImageReference, pushRef, rawRef string, options types.ImagePushOptions, platMC platforms.MatchComparer) error {
	ref := parsedReference.String()
	refDomain := parsedReference.Domain

	// In order to push images where most layers are the same but the
	// repository name is different, it is necessary to refresh the
	// PushTracker. Otherwise, the MANIFEST_BLOB_UNKNOWN error will occur due
	// to the registry not creating the corresponding layer link file,
	// resulting in the failure of the entire image push.
	pushTracker := docker.NewInMemoryTracker()

	pushFunc := func(r remotes.Resolver) error {
		return push.Push(ctx, client, r, pushTracker, options.Stdout, pushRef, ref, platMC, options.AllowNondistributableArtifacts, options.Quiet)
	}

	var dOpts []dockerconfigresolver.Opt
	if options.GOptions.InsecureRegistry {
		log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", refDomain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(options.GOptions.HostsDir))

	ho, err := dockerconfigresolver.NewHostOptions(ctx, refDomain, dOpts...)
	if err != nil {
		return err
	}

	resolverOpts := docker.ResolverOptions{
		Tracker: pushTracker,
		Hosts:   dockerconfig.ConfigureHosts(ctx, *ho),
	}

	resolver := docker.NewResolver(resolverOpts)
	if err = pushFunc(resolver); err != nil {
		// In some circumstance (e.g. people just use 80 port to support pure http), the error will contain message like "dial tcp <port>: connection refused"
		if !errors.Is(err, http.ErrSchemeMismatch) && !errutil.IsErrConnectionRefused(err) {
			return err
		}
		if options.GOptions.InsecureRegistry {
			log.G(ctx).WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", refDomain)
			dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
			// Rebuild the resolver rather than calling dockerconfigresolver.New, which would fall
			// back to the process-wide dockerconfigresolver.PushTracker. That tracker is keyed by
			// digest, not by reference, so a second push of an already-pushed digest short-circuits
			// with ErrAlreadyExists and its tag is never written to the registry.
			ho, err = dockerconfigresolver.NewHostOptions(ctx, refDomain, dOpts...)
			if err != nil {
				return err
			}
			resolverOpts.Hosts = dockerconfig.ConfigureHosts(ctx, *ho)
			return pushFunc(docker.NewResolver(resolverOpts))
		}
		log.G(ctx).WithError(err).Errorf("server %q does not seem to support HTTPS", refDomain)
		log.G(ctx).Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
		return err
	}
	return nil
}
