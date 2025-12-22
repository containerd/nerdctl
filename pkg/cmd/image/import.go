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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/transfer"
	tarchive "github.com/containerd/containerd/v2/core/transfer/archive"
	transferimage "github.com/containerd/containerd/v2/core/transfer/image"
	"github.com/containerd/containerd/v2/pkg/archive/compression"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/transferutil"
)

func Import(ctx context.Context, client *containerd.Client, options types.ImageImportOptions) (string, error) {
	prefix := options.Reference
	if prefix == "" {
		prefix = fmt.Sprintf("import-%s", time.Now().Format("2006-01-02"))
	}

	parsed, err := referenceutil.Parse(prefix)
	if err != nil {
		return "", err
	}
	imageName := parsed.String()

	platUnpack := platforms.DefaultSpec()
	var opts []transferimage.StoreOpt
	if options.Platform != "" {
		p, err := platforms.Parse(options.Platform)
		if err != nil {
			return "", err
		}
		platUnpack = p
		opts = append(opts, transferimage.WithPlatforms(platUnpack))
	}

	opts = append(opts, transferimage.WithUnpack(platUnpack, options.GOptions.Snapshotter))
	opts = append(opts, transferimage.WithDigestRef(imageName, true, true))

	var r io.ReadCloser
	if rc, ok := options.Stdin.(io.ReadCloser); ok {
		r = rc
	} else {
		r = io.NopCloser(options.Stdin)
	}

	converted, cleanup, err := ensureOCIArchive(ctx, client, r, options, prefix)
	if err != nil {
		return "", err
	}
	defer cleanup()

	iis := tarchive.NewImageImportStream(converted, "")
	is := transferimage.NewStore("", opts...)

	pf, done := transferutil.ProgressHandler(ctx, os.Stderr)
	defer done()

	if err := client.Transfer(ctx, iis, is, transfer.WithProgress(pf)); err != nil {
		return "", err
	}

	return imageName, nil
}

func ensureOCIArchive(ctx context.Context, client *containerd.Client, r io.ReadCloser, options types.ImageImportOptions, prefix string) (io.ReadCloser, func(), error) {
	buf := &bytes.Buffer{}
	tee := io.TeeReader(r, buf)

	isStandardArchive, err := detectStandardImageArchive(tee)
	if err != nil {
		return nil, func() {}, err
	}

	combined := io.NopCloser(io.MultiReader(buf, r))
	if isStandardArchive {
		return combined, func() { r.Close() }, nil
	}

	converted, err := convertRootfsToOCIArchive(ctx, client, combined, options, prefix)
	if err != nil {
		r.Close()
		return nil, func() {}, err
	}

	cleanup := func() {
		r.Close()
		if converted != nil {
			converted.Close()
		}
	}

	return converted, cleanup, nil
}

func detectStandardImageArchive(r io.Reader) (bool, error) {
	tr := tar.NewReader(r)
	const maxHeadersToCheck = 10

	for i := 0; i < maxHeadersToCheck; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}

		name := pathpkg.Clean(hdr.Name)
		if name == "manifest.json" || name == ocispec.ImageLayoutFile {
			return true, nil
		}
	}
	return false, nil
}

func convertRootfsToOCIArchive(ctx context.Context, client *containerd.Client, r io.ReadCloser, options types.ImageImportOptions, prefix string) (io.ReadCloser, error) {
	defer r.Close()

	ctx, done, err := client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return nil, err
	}
	defer done(ctx)

	decomp, err := compression.DecompressStream(r)
	if err != nil {
		return nil, err
	}
	defer decomp.Close()

	cs := client.ContentStore()
	ref := randomRef("import-layer-")
	w, err := content.OpenWriter(ctx, cs, content.WithRef(ref))
	if err != nil {
		return nil, err
	}
	defer w.Close()

	if err := w.Truncate(0); err != nil {
		return nil, err
	}

	layerDigest, diffID, layerSize, err := compressAndWriteLayer(ctx, w, decomp)
	if err != nil {
		return nil, err
	}

	imgConfig, configDigest, err := buildImageConfig(diffID, options)
	if err != nil {
		return nil, err
	}

	layerContent, err := readLayerContent(ctx, cs, layerDigest, layerSize)
	if err != nil {
		return nil, err
	}

	return buildDockerArchive(imgConfig, configDigest, layerContent, layerDigest, prefix)
}

func compressAndWriteLayer(ctx context.Context, w content.Writer, r io.Reader) (digest.Digest, digest.Digest, int64, error) {
	digester := digest.Canonical.Digester()
	tee := io.TeeReader(r, digester.Hash())
	pr, pw := io.Pipe()
	gz := gzip.NewWriter(pw)

	doneCh := make(chan error, 1)
	go func() {
		defer func() {
			_ = gz.Close()
		}()

		if _, err := io.Copy(gz, tee); err != nil {
			doneCh <- err
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
		return "", "", 0, err
	}
	if err := <-doneCh; err != nil {
		return "", "", 0, err
	}

	diffID := digester.Digest()
	labels := map[string]string{
		"containerd.io/uncompressed": diffID.String(),
	}
	if err := w.Commit(ctx, n, "", content.WithLabels(labels)); err != nil && !errdefs.IsAlreadyExists(err) {
		return "", "", 0, err
	}

	return w.Digest(), diffID, n, nil
}

func buildImageConfig(diffID digest.Digest, options types.ImageImportOptions) ([]byte, digest.Digest, error) {
	ociplat := platforms.DefaultSpec()
	if options.Platform != "" {
		if p, err := platforms.Parse(options.Platform); err == nil {
			ociplat = p
		}
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

	configJSON, err := json.Marshal(imgConfig)
	if err != nil {
		return nil, "", err
	}
	return configJSON, digest.FromBytes(configJSON), nil
}

func readLayerContent(ctx context.Context, cs content.Store, layerDigest digest.Digest, size int64) ([]byte, error) {
	ra, err := cs.ReaderAt(ctx, ocispec.Descriptor{Digest: layerDigest, Size: size})
	if err != nil {
		return nil, err
	}
	defer ra.Close()

	layerContent := make([]byte, size)
	if _, err := ra.ReadAt(layerContent, 0); err != nil {
		return nil, err
	}
	return layerContent, nil
}

func buildDockerArchive(configJSON []byte, configDigest digest.Digest, layerContent []byte, layerDigest digest.Digest, prefix string) (io.ReadCloser, error) {
	layerFileName := layerDigest.Encoded() + ".tar.gz"
	configFileName := configDigest.Encoded() + ".json"

	var repoTags []string
	if parsed, err := referenceutil.Parse(prefix); err == nil && parsed.String() != "" {
		repoTags = []string{parsed.String()}
	}

	dockerManifest := []struct {
		Config   string   `json:"Config"`
		RepoTags []string `json:"RepoTags,omitempty"`
		Layers   []string `json:"Layers"`
	}{{
		Config:   configFileName,
		RepoTags: repoTags,
		Layers:   []string{layerFileName},
	}}

	dockerManifestJSON, err := json.Marshal(dockerManifest)
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	files := []struct {
		name    string
		content []byte
	}{
		{"manifest.json", dockerManifestJSON},
		{configFileName, configJSON},
		{layerFileName, layerContent},
	}

	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.name,
			Mode: 0644,
			Size: int64(len(f.content)),
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.content); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return io.NopCloser(buf), nil
}

func randomRef(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + base64.RawURLEncoding.EncodeToString(b[:])
}
