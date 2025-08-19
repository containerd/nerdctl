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
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/pkg/archive/compression"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Import(ctx context.Context, client *containerd.Client, options types.ImageImportOptions) (string, error) {
	img, err := importRootfs(ctx, client, options.GOptions.Snapshotter, options)
	if err != nil {
		return "", err
	}
	return img.Name, nil
}

func importRootfs(ctx context.Context, client *containerd.Client, snapshotter string, options types.ImageImportOptions) (images.Image, error) {
	var zero images.Image

	ctx, done, err := client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return zero, err
	}
	defer done(ctx)

	if options.Stdin == nil {
		return zero, fmt.Errorf("no input stream provided")
	}
	decomp, err := compression.DecompressStream(options.Stdin)
	if err != nil {
		return zero, err
	}
	defer decomp.Close()

	cs := client.ContentStore()

	ref := randomRef("import-rootfs-")
	w, err := content.OpenWriter(ctx, cs, content.WithRef(ref))
	if err != nil {
		return zero, err
	}
	defer w.Close()
	if err := w.Truncate(0); err != nil {
		return zero, err
	}

	digester := digest.Canonical.Digester()
	tee := io.TeeReader(decomp, digester.Hash())
	pr, pw := io.Pipe()
	gz := gzip.NewWriter(pw)
	doneCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(gz, tee)
		if err != nil {
			doneCh <- err
			_ = gz.Close()
			_ = pw.CloseWithError(err)
			return
		}
		if err := gz.Close(); err != nil {
			doneCh <- err
			_ = pw.CloseWithError(err)
			return
		}
		doneCh <- pw.Close()
	}()

	n, err := io.Copy(w, pr)
	if err != nil {
		return zero, err
	}
	if err := <-doneCh; err != nil {
		return zero, err
	}

	diffID := digester.Digest()
	labels := map[string]string{
		"containerd.io/uncompressed": diffID.String(),
	}
	if err := w.Commit(ctx, n, "", content.WithLabels(labels)); err != nil && !errdefs.IsAlreadyExists(err) {
		return zero, err
	}
	layerDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2LayerGzip,
		Digest:    w.Digest(),
		Size:      n,
	}

	ociplat := platforms.DefaultSpec()
	if options.Platform != "" {
		p, err := platforms.Parse(options.Platform)
		if err != nil {
			return zero, err
		}
		ociplat = p
	}

	created := time.Now().UTC()
	imgConfig := ocispec.Image{
		Platform: ocispec.Platform{
			Architecture: ociplat.Architecture,
			OS:           ociplat.OS,
			OSVersion:    ociplat.OSVersion,
			Variant:      ociplat.Variant,
		},
		Created: &created,
		Config:  ocispec.ImageConfig{},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{diffID},
		},
		History: []ocispec.History{{
			Created: &created,
			Comment: options.Message,
		}},
	}

	manifestDesc, _, err := writeConfigAndManifest(ctx, cs, snapshotter, imgConfig, []ocispec.Descriptor{layerDesc})
	if err != nil {
		return zero, err
	}

	storedName := options.Reference
	if storedName == "" {
		storedName = manifestDesc.Digest.String()
	} else if refParsed, err := referenceutil.Parse(storedName); err == nil {
		if refParsed.ExplicitTag == "" {
			storedName = refParsed.FamiliarName() + ":latest"
		}
		if p2, err := referenceutil.Parse(storedName); err == nil {
			storedName = p2.String()
		}
	}
	name := storedName

	img := images.Image{
		Name:      name,
		Target:    manifestDesc,
		CreatedAt: time.Now(),
	}
	if _, err := client.ImageService().Update(ctx, img); err != nil {
		if !errdefs.IsNotFound(err) {
			return zero, err
		}
		if _, err := client.ImageService().Create(ctx, img); err != nil {
			return zero, err
		}
	}

	cimg := containerd.NewImage(client, img)
	if err := cimg.Unpack(ctx, snapshotter); err != nil {
		return zero, err
	}
	return img, nil
}

func randomRef(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + base64.RawURLEncoding.EncodeToString(b[:])
}

func writeConfigAndManifest(ctx context.Context, cs content.Store, snapshotter string, config ocispec.Image, layers []ocispec.Descriptor) (ocispec.Descriptor, digest.Digest, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return ocispec.Descriptor{}, "", err
	}
	configDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Config,
		Digest:    digest.FromBytes(configJSON),
		Size:      int64(len(configJSON)),
	}

	gcLabel := map[string]string{}
	if len(config.RootFS.DiffIDs) > 0 && snapshotter != "" {
		gcLabel[fmt.Sprintf("containerd.io/gc.ref.snapshot.%s", snapshotter)] = identity.ChainID(config.RootFS.DiffIDs).String()
	}
	if err := content.WriteBlob(ctx, cs, configDesc.Digest.String(), bytes.NewReader(configJSON), configDesc, content.WithLabels(gcLabel)); err != nil && !errdefs.IsAlreadyExists(err) {
		return ocispec.Descriptor{}, "", err
	}

	manifest := struct {
		MediaType string `json:"mediaType,omitempty"`
		ocispec.Manifest
	}{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Manifest: ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			Config:    configDesc,
			Layers:    layers,
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, "", err
	}
	manifestDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Digest:    digest.FromBytes(manifestJSON),
		Size:      int64(len(manifestJSON)),
	}

	refLabels := map[string]string{
		"containerd.io/gc.ref.content.0": configDesc.Digest.String(),
	}
	for i, l := range layers {
		refLabels[fmt.Sprintf("containerd.io/gc.ref.content.%d", i+1)] = l.Digest.String()
	}
	if err := content.WriteBlob(ctx, cs, manifestDesc.Digest.String(), bytes.NewReader(manifestJSON), manifestDesc, content.WithLabels(refLabels)); err != nil && !errdefs.IsAlreadyExists(err) {
		return ocispec.Descriptor{}, "", err
	}

	return manifestDesc, configDesc.Digest, nil
}
