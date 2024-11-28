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
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
)

const (
	emptyDigest = digest.Digest("")
)

type squashImage struct {
	ClientImage containerd.Image
	Config      ocispec.Image
	Image       images.Image
	Manifest    *ocispec.Manifest
}

type squashRuntime struct {
	opt types.ImageSquashOptions

	client    *containerd.Client
	namespace string

	differ       containerd.DiffService
	imageStore   images.Store
	contentStore content.Store
	snapshotter  snapshots.Snapshotter
}

func (sr *squashRuntime) initImage(ctx context.Context) (*squashImage, error) {
	containerImage, err := sr.imageStore.Get(ctx, sr.opt.SourceImageRef)
	if err != nil {
		return &squashImage{}, err
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
		ClientImage: clientImage,
		Config:      config,
		Image:       containerImage,
		Manifest:    manifest,
	}
	return resImage, err
}

func (sr *squashRuntime) generateSquashLayer(image *squashImage) ([]ocispec.Descriptor, error) {
	// get the layer descriptors by the layer digest
	if sr.opt.SquashLayerDigest != "" {
		find := false
		var res []ocispec.Descriptor
		for _, layer := range image.Manifest.Layers {
			if layer.Digest.String() == sr.opt.SquashLayerDigest {
				find = true
			}
			if find {
				res = append(res, layer)
			}
		}
		if !find {
			return nil, fmt.Errorf("layer digest %s not found in the image: %w", sr.opt.SquashLayerDigest, errdefs.ErrNotFound)
		}
		return res, nil
	}

	// get the layer descriptors by the layer count
	if sr.opt.SquashLayerCount > 1 && sr.opt.SquashLayerCount <= len(image.Manifest.Layers) {
		return image.Manifest.Layers[len(image.Manifest.Layers)-sr.opt.SquashLayerCount:], nil
	}

	return nil, fmt.Errorf("invalid squash option: %w", errdefs.ErrInvalidArgument)
}

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
	orginalConfig, _, err := imgutil.ReadImageConfig(ctx, image.ClientImage) // aware of img.platform
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

func (sr *squashRuntime) createSquashImage(ctx context.Context, img images.Image) (images.Image, error) {
	newImg, err := sr.imageStore.Update(ctx, img)
	log.G(ctx).Infof("updated new squashImage %s", img.Name)
	if err != nil {
		// if err is `not found` in the message then create the squashImage, otherwise return the error
		if !errdefs.IsNotFound(err) {
			return newImg, fmt.Errorf("failed to update new squashImage %s: %w", img.Name, err)
		}
		if _, err := sr.imageStore.Create(ctx, img); err != nil {
			return newImg, fmt.Errorf("failed to create new squashImage %s: %w", img.Name, err)
		}
		log.G(ctx).Infof("created new squashImage %s", img.Name)
	}
	return newImg, nil
}

// generateCommitImageConfig returns commit oci image config based on the container's image.
func (sr *squashRuntime) generateCommitImageConfig(ctx context.Context, baseConfig ocispec.Image, diffID digest.Digest) (ocispec.Image, error) {
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
			CreatedBy:  "",
			Author:     author,
			Comment:    comment,
			EmptyLayer: false,
		}),
	}, nil
}

// Squash will squash the image with the given options.
func Squash(ctx context.Context, client *containerd.Client, option types.ImageSquashOptions) error {
	sr := newSquashRuntime(client, option)
	ctx = namespaces.WithNamespace(ctx, sr.namespace)
	// init squashImage
	image, err := sr.initImage(ctx)
	if err != nil {
		return err
	}
	// generate squash layers
	sLayers, err := sr.generateSquashLayer(image)
	if err != nil {
		return err
	}
	remainingLayerCount := len(image.Manifest.Layers) - len(sLayers)
	// Don't gc me and clean the dirty data after 1 hour!
	ctx, done, err := sr.client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to create lease for squash: %w", err)
	}
	defer done(ctx)

	// generate remaining base squashImage config
	baseImage, err := sr.generateBaseImageConfig(ctx, image, remainingLayerCount)
	if err != nil {
		return err
	}
	diffLayerDesc, diffID, _, err := sr.applyDiffLayer(ctx, baseImage, sr.snapshotter, sLayers)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to apply diff layer")
		return err
	}
	// generate commit image config
	imageConfig, err := sr.generateCommitImageConfig(ctx, baseImage, diffID)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to generate commit image config")
		return fmt.Errorf("failed to generate commit image config: %w", err)
	}
	commitManifestDesc, _, err := sr.writeContentsForImage(ctx, sr.opt.GOptions.Snapshotter, imageConfig, image.Manifest.Layers[:remainingLayerCount], diffLayerDesc)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to write contents for image")
		return err
	}
	nimg := images.Image{
		Name:      sr.opt.TargetImageName,
		Target:    commitManifestDesc,
		UpdatedAt: time.Now(),
	}
	_, err = sr.createSquashImage(ctx, nimg)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to create squash image")
		return err
	}
	cimg := containerd.NewImage(sr.client, nimg)
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

// copied from github.com/containerd/containerd/rootfs/apply.go
func uniquePart() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.Nanosecond(), base64.URLEncoding.EncodeToString(b[:]))
}
