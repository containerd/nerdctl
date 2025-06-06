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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/rootfs"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
)

const (
	emptyDigest = digest.Digest("")
)

// squashImage is the image for squash operation
type squashImage struct {
	clientImage containerd.Image
	config      ocispec.Image
	image       images.Image
	manifest    *ocispec.Manifest
}

// squashRuntime is the runtime for squash operation
type squashRuntime struct {
	opt types.ImageSquashOptions

	client    *containerd.Client
	namespace string

	differ       containerd.DiffService
	imageStore   images.Store
	contentStore content.Store
	snapshotter  snapshots.Snapshotter
}

// initImage initializes the squashImage based on the source image reference
func (sr *squashRuntime) initImage(ctx context.Context) (*squashImage, error) {
	containerImage, err := sr.imageStore.Get(ctx, sr.opt.SourceImageRef)
	if err != nil {
		return &squashImage{}, err
	}
	targetDesc := containerImage.Target
	if !images.IsManifestType(targetDesc.MediaType) {
		return &squashImage{}, fmt.Errorf("only manifest type is supported :%w", errdefs.ErrInvalidArgument)
	}

	clientImage := containerd.NewImage(sr.client, containerImage)
	manifest, _, err := imgutil.ReadManifest(ctx, clientImage)
	if err != nil {
		return &squashImage{}, err
	}
	config, _, err := imgutil.ReadImageConfig(ctx, clientImage)
	if err != nil {
		return &squashImage{}, err
	}
	resImage := &squashImage{
		clientImage: clientImage,
		config:      config,
		image:       containerImage,
		manifest:    manifest,
	}
	return resImage, err
}

// generateSquashLayer generates the squash layer based on the given options
func (sr *squashRuntime) generateSquashLayer(image *squashImage) ([]ocispec.Descriptor, error) {
	// get the layer descriptors by the layer count
	if sr.opt.SquashLayerLastN > 1 && sr.opt.SquashLayerLastN <= len(image.manifest.Layers) {
		return image.manifest.Layers[len(image.manifest.Layers)-sr.opt.SquashLayerLastN:], nil
	}

	return nil, fmt.Errorf("invalid squash option: %w", errdefs.ErrInvalidArgument)
}

// applyLayersToSnapshot applies the layers to the snapshot
func (sr *squashRuntime) applyLayersToSnapshot(ctx context.Context, mount []mount.Mount, layers []ocispec.Descriptor) error {
	for _, layer := range layers {
		if _, err := sr.differ.Apply(ctx, layer, mount); err != nil {
			return err
		}
	}
	return nil
}

// createDiff creates a diff from the snapshot
func (sr *squashRuntime) createDiff(ctx context.Context, snapshotName string) (ocispec.Descriptor, digest.Digest, error) {
	newDesc, err := rootfs.CreateDiff(ctx, snapshotName, sr.snapshotter, sr.differ)
	if err != nil {
		return ocispec.Descriptor{}, "", err
	}
	info, err := sr.contentStore.Info(ctx, newDesc.Digest)
	if err != nil {
		return ocispec.Descriptor{}, "", err
	}
	diffIDStr, ok := info.Labels["containerd.io/uncompressed"]
	if !ok {
		return ocispec.Descriptor{}, "", fmt.Errorf("invalid differ response with no diffID")
	}
	diffID, err := digest.Parse(diffIDStr)
	if err != nil {
		return ocispec.Descriptor{}, "", err
	}
	return ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2LayerGzip,
		Digest:    newDesc.Digest,
		Size:      info.Size,
	}, diffID, nil
}

func (sr *squashRuntime) generateBaseImageConfig(ctx context.Context, image *squashImage, remainingLayerCount int) (ocispec.Image, error) {
	// generate squash squashImage config
	orginalConfig, _, err := imgutil.ReadImageConfig(ctx, image.clientImage) // aware of img.platform
	if err != nil {
		return ocispec.Image{}, err
	}

	var history []ocispec.History
	var count int
	for _, h := range orginalConfig.History {
		// if empty layer, add to history, be careful with the last layer that is empty
		if h.EmptyLayer {
			history = append(history, h)
			continue
		}
		// if not empty layer, add to history, check if count+1 <= remainingLayerCount to see if we need to add more
		if count+1 <= remainingLayerCount {
			history = append(history, h)
			count++
		} else {
			break
		}
	}
	cTime := time.Now()
	return ocispec.Image{
		Created:  &cTime,
		Author:   orginalConfig.Author,
		Platform: orginalConfig.Platform,
		Config:   orginalConfig.Config,
		RootFS: ocispec.RootFS{
			Type:    orginalConfig.RootFS.Type,
			DiffIDs: orginalConfig.RootFS.DiffIDs[:remainingLayerCount],
		},
		History: history,
	}, nil
}

// writeContentsForImage will commit oci image config and manifest into containerd's content store.
func (sr *squashRuntime) writeContentsForImage(ctx context.Context, snName string, newConfig ocispec.Image,
	baseImageLayers []ocispec.Descriptor, diffLayerDesc ocispec.Descriptor) (ocispec.Descriptor, digest.Digest, error) {
	newConfigJSON, err := json.Marshal(newConfig)
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	configDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Config,
		Digest:    digest.FromBytes(newConfigJSON),
		Size:      int64(len(newConfigJSON)),
	}

	layers := append(baseImageLayers, diffLayerDesc)

	newMfst := struct {
		MediaType string `json:"mediaType,omitempty"`
		ocispec.Manifest
	}{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Manifest: ocispec.Manifest{
			Versioned: specs.Versioned{
				SchemaVersion: 2,
			},
			Config: configDesc,
			Layers: layers,
		},
	}

	newMfstJSON, err := json.MarshalIndent(newMfst, "", "    ")
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	newMfstDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Digest:    digest.FromBytes(newMfstJSON),
		Size:      int64(len(newMfstJSON)),
	}

	// new manifest should reference the layers and config content
	labels := map[string]string{
		"containerd.io/gc.ref.content.0": configDesc.Digest.String(),
	}
	for i, l := range layers {
		labels[fmt.Sprintf("containerd.io/gc.ref.content.%d", i+1)] = l.Digest.String()
	}

	err = content.WriteBlob(ctx, sr.contentStore, newMfstDesc.Digest.String(), bytes.NewReader(newMfstJSON), newMfstDesc, content.WithLabels(labels))
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	// config should reference to snapshotter
	labelOpt := content.WithLabels(map[string]string{
		fmt.Sprintf("containerd.io/gc.ref.snapshot.%s", snName): identity.ChainID(newConfig.RootFS.DiffIDs).String(),
	})
	err = content.WriteBlob(ctx, sr.contentStore, configDesc.Digest.String(), bytes.NewReader(newConfigJSON), configDesc, labelOpt)
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}
	return newMfstDesc, configDesc.Digest, nil
}

// createSquashImage creates a new squashImage in the image store.
func (sr *squashRuntime) createSquashImage(ctx context.Context, img images.Image) (images.Image, error) {
	newImg, err := sr.imageStore.Update(ctx, img)
	if err != nil {
		// if err is `not found` in the message then create the squashImage, otherwise return the error
		if !errdefs.IsNotFound(err) {
			return newImg, fmt.Errorf("failed to update new squashImage %s: %w", img.Name, err)
		}
		if _, err := sr.imageStore.Create(ctx, img); err != nil {
			return newImg, fmt.Errorf("failed to create new squashImage %s: %w", img.Name, err)
		}
	}
	return newImg, nil
}

// generateCommitImageConfig returns commit oci image config based on the container's image.
func (sr *squashRuntime) generateCommitImageConfig(ctx context.Context, baseImg images.Image, baseConfig ocispec.Image, diffID digest.Digest) (ocispec.Image, error) {
	createdTime := time.Now()
	arch := baseConfig.Architecture
	if arch == "" {
		arch = runtime.GOARCH
		log.G(ctx).Warnf("assuming arch=%q", arch)
	}
	os := baseConfig.OS
	if os == "" {
		os = runtime.GOOS
		log.G(ctx).Warnf("assuming os=%q", os)
	}
	author := strings.TrimSpace(sr.opt.Author)
	if author == "" {
		author = baseConfig.Author
	}
	comment := strings.TrimSpace(sr.opt.Message)

	baseImageDigest := strings.Split(baseImg.Target.Digest.String(), ":")[1][:12]
	return ocispec.Image{
		Platform: ocispec.Platform{
			Architecture: arch,
			OS:           os,
		},

		Created: &createdTime,
		Author:  author,
		Config:  baseConfig.Config,
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: append(baseConfig.RootFS.DiffIDs, diffID),
		},
		History: append(baseConfig.History, ocispec.History{
			Created:    &createdTime,
			CreatedBy:  fmt.Sprintf("squash from %s", baseImageDigest),
			Author:     author,
			Comment:    comment,
			EmptyLayer: false,
		}),
	}, nil
}

// Squash will squash the image with the given options.
func Squash(ctx context.Context, client *containerd.Client, option types.ImageSquashOptions) error {
	var srcName string
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if srcName == "" {
				srcName = found.Image.Name
			}
			return nil
		},
	}
	matchCount, err := walker.Walk(ctx, option.SourceImageRef)
	if err != nil {
		return err
	}
	if matchCount < 1 {
		return fmt.Errorf("%s: not found", option.SourceImageRef)
	}
	if matchCount > 1 {
		return fmt.Errorf("multiple imageRef found with provided prefix: %s", option.SourceImageRef)
	}

	option.SourceImageRef = srcName
	sr := newSquashRuntime(client, option)
	ctx = namespaces.WithNamespace(ctx, sr.namespace)
	// init squashImage
	img, err := sr.initImage(ctx)
	if err != nil {
		return err
	}
	// generate squash layers
	sLayers, err := sr.generateSquashLayer(img)
	if err != nil {
		return err
	}
	remainingLayerCount := len(img.manifest.Layers) - len(sLayers)
	// Don't gc me and clean the dirty data after 1 hour!
	ctx, done, err := sr.client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to create lease for squash: %w", err)
	}
	defer done(ctx)

	// generate remaining base squashImage config
	baseImage, err := sr.generateBaseImageConfig(ctx, img, remainingLayerCount)
	if err != nil {
		return err
	}
	diffLayerDesc, diffID, _, err := sr.applyDiffLayer(ctx, baseImage, sr.snapshotter, sLayers)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to apply diff layer")
		return err
	}
	// generate commit image config
	imageConfig, err := sr.generateCommitImageConfig(ctx, img.image, baseImage, diffID)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to generate commit image config")
		return fmt.Errorf("failed to generate commit image config: %w", err)
	}
	commitManifestDesc, _, err := sr.writeContentsForImage(ctx, sr.opt.GOptions.Snapshotter, imageConfig, img.manifest.Layers[:remainingLayerCount], diffLayerDesc)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to write contents for image")
		return err
	}
	nImg := images.Image{
		Name:      sr.opt.TargetImageName,
		Target:    commitManifestDesc,
		UpdatedAt: time.Now(),
	}
	_, err = sr.createSquashImage(ctx, nImg)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to create squash image")
		return err
	}
	cimg := containerd.NewImage(sr.client, nImg)
	if err := cimg.Unpack(ctx, sr.opt.GOptions.Snapshotter, containerd.WithSnapshotterPlatformCheck()); err != nil {
		log.G(ctx).WithError(err).Error("failed to unpack squash image")
		return err
	}
	return nil
}

// applyDiffLayer will apply diff layer content created by createDiff into the snapshotter.
func (sr *squashRuntime) applyDiffLayer(ctx context.Context, baseImg ocispec.Image, sn snapshots.Snapshotter, layers []ocispec.Descriptor) (
	diffLayerDesc ocispec.Descriptor, diffID digest.Digest, snapshotID string, retErr error) {
	var (
		key    = uniquePart()
		parent = identity.ChainID(baseImg.RootFS.DiffIDs).String()
	)

	m, err := sn.Prepare(ctx, key, parent)
	if err != nil {
		return diffLayerDesc, diffID, snapshotID, err
	}

	defer func() {
		if retErr != nil {
			// NOTE: the snapshotter should be hold by lease. Even
			// if the cleanup fails, the containerd gc can delete it.
			if err := sn.Remove(ctx, key); err != nil {
				log.G(ctx).Warnf("failed to cleanup aborted apply %s: %s", key, err)
			}
		}
	}()

	err = sr.applyLayersToSnapshot(ctx, m, layers)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to apply layers to snapshot %s", key)
		return diffLayerDesc, diffID, snapshotID, err
	}
	diffLayerDesc, diffID, err = sr.createDiff(ctx, key)
	if err != nil {
		return diffLayerDesc, diffID, snapshotID, fmt.Errorf("failed to export layer: %w", err)
	}

	// commit snapshot
	snapshotID = identity.ChainID(append(baseImg.RootFS.DiffIDs, diffID)).String()

	if err = sn.Commit(ctx, snapshotID, key); err != nil {
		if errdefs.IsAlreadyExists(err) {
			return diffLayerDesc, diffID, snapshotID, nil
		}
		return diffLayerDesc, diffID, snapshotID, err
	}
	return diffLayerDesc, diffID, snapshotID, nil
}

func newSquashRuntime(client *containerd.Client, option types.ImageSquashOptions) *squashRuntime {
	return &squashRuntime{
		opt:          option,
		client:       client,
		namespace:    option.GOptions.Namespace,
		differ:       client.DiffService(),
		imageStore:   client.ImageService(),
		contentStore: client.ContentStore(),
		snapshotter:  client.SnapshotService(option.GOptions.Snapshotter),
	}
}

// copied from https://github.com/containerd/containerd/blob/89623f28b87a6004d4b785663257362d1658a729/rootfs/apply.go#L106
func uniquePart() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.Nanosecond(), base64.URLEncoding.EncodeToString(b[:]))
}
