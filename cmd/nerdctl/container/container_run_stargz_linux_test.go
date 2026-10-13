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

package container

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	digest "github.com/opencontainers/go-digest"
	imagespecversioned "github.com/opencontainers/image-spec/specs-go"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"
	"github.com/containerd/platforms"
	"github.com/containerd/stargz-snapshotter/estargz"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

func TestRunStargz(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		nerdtest.Stargz,
		require.Amd64,
		require.Not(nerdtest.Docker),
	)

	testCase.Command = test.Command("--snapshotter=stargz", "run", "--quiet", "--rm", testutil.FedoraESGZImage, "ls", "/.stargz-snapshotter")

	testCase.Expected = test.Expects(0, nil, nil)

	testCase.Run(t)
}

// createInvalidESGZImage creates an eStargz image from the first layer of srcRef (an eStargz image) with an extra
// file added to the layer, while keeping the original image config. As the diffID in the config (the one of srcRef)
// doesn't match the modified layer, the resulting image has the same ChainID as srcRef but different contents and
// TOC digest. The image is stored in containerd as newRef.
func createInvalidESGZImage(ctx context.Context, t tig.T, client *containerd.Client, srcRef, newRef string) {
	cs := client.ContentStore()
	srcImg, err := client.GetImage(ctx, srcRef)
	assert.NilError(t, err)
	srcManifest, err := images.Manifest(ctx, cs, srcImg.Target(), platforms.Default())
	assert.NilError(t, err)
	layer, err := content.ReadBlob(ctx, cs, srcManifest.Layers[0])
	assert.NilError(t, err)

	var modified bytes.Buffer
	tw := tar.NewWriter(&modified)
	gz, err := gzip.NewReader(bytes.NewReader(layer))
	assert.NilError(t, err)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		assert.NilError(t, err)
		assert.NilError(t, tw.WriteHeader(h))
		_, err = io.Copy(tw, tr)
		assert.NilError(t, err)
	}
	contents := "MODIFIED"
	assert.NilError(t, tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "/modified.txt", Size: int64(len(contents))}))
	_, err = tw.Write([]byte(contents))
	assert.NilError(t, err)
	assert.NilError(t, tw.Close())

	modifiedData := modified.Bytes()
	esgz, err := estargz.Build(io.NewSectionReader(bytes.NewReader(modifiedData), 0, int64(len(modifiedData))))
	assert.NilError(t, err)
	defer esgz.Close()
	layerData, err := io.ReadAll(esgz)
	assert.NilError(t, err)
	layerDesc := imagespec.Descriptor{
		MediaType:   imagespec.MediaTypeImageLayerGzip,
		Digest:      digest.FromBytes(layerData),
		Size:        int64(len(layerData)),
		Annotations: map[string]string{estargz.TOCJSONDigestAnnotation: esgz.TOCDigest().String()},
	}
	assert.NilError(t, content.WriteBlob(ctx, cs, "layer-invalid", bytes.NewReader(layerData), layerDesc))

	srcConfig, err := images.Config(ctx, cs, srcImg.Target(), platforms.Default())
	assert.NilError(t, err)
	manifest := &imagespec.Manifest{
		Versioned: imagespecversioned.Versioned{SchemaVersion: 2},
		MediaType: imagespec.MediaTypeImageManifest,
		Config:    srcConfig,
		Layers:    []imagespec.Descriptor{layerDesc},
	}
	manifestBlob, err := json.Marshal(manifest)
	assert.NilError(t, err)
	manifestDesc := imagespec.Descriptor{
		MediaType: imagespec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBlob),
		Size:      int64(len(manifestBlob)),
	}
	labels := map[string]string{}
	for i, d := range []imagespec.Descriptor{layerDesc, srcConfig} {
		labels[fmt.Sprintf("containerd.io/gc.ref.content.%d", i)] = d.Digest.String()
	}
	assert.NilError(t, content.WriteBlob(ctx, cs, "manifest-invalid", bytes.NewReader(manifestBlob), manifestDesc, content.WithLabels(labels)))
	_, err = client.ImageService().Create(ctx, images.Image{Name: newRef, Target: manifestDesc})
	assert.NilError(t, err)
}

func newTestContainerdClient(helpers test.Helpers) *containerd.Client {
	addr := defaults.DefaultAddress
	if rootlessutil.IsRootless() {
		stateDir, err := rootlessutil.RootlessKitStateDir()
		assert.NilError(helpers.T(), err)
		childPid, err := rootlessutil.RootlessKitChildPid(stateDir)
		assert.NilError(helpers.T(), err)
		addr = filepath.Join("/proc", fmt.Sprintf("%d", childPid), "root", defaults.DefaultAddress)
	}
	client, err := containerd.New(addr, containerd.WithDefaultNamespace(string(helpers.Read(nerdtest.Namespace))))
	assert.NilError(helpers.T(), err)
	return client
}

func TestPullStargzInvalidChainID(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		nerdtest.Stargz,
		nerdtest.Registry,
		require.Amd64,
		require.Not(nerdtest.Docker),
	)

	var reg *registry.Server

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		reg = nerdtest.RegistryWithNoAuth(data, helpers, 0, false)
		reg.Setup(data, helpers)
		base := fmt.Sprintf("127.0.0.1:%d/alpine", reg.Port)
		esgz := base + ":esgz"
		invalid := base + ":esgz-invalid"

		// A valid eStargz image
		helpers.Ensure("pull", "--quiet", testutil.AlpineImage)
		helpers.Ensure("image", "convert", "--estargz", "--oci", testutil.AlpineImage, esgz)
		helpers.Ensure("push", esgz)

		// An eStargz image whose layer doesn't match the diffID in the config
		client := newTestContainerdClient(helpers)
		defer client.Close()
		createInvalidESGZImage(context.Background(), helpers.T(), client, esgz, invalid)
		helpers.Ensure("push", invalid)

		// Remove local copies so that the following pulls fetch from the registry
		helpers.Ensure("rmi", "-f", esgz, invalid)

		data.Labels().Set("esgz", esgz)
		data.Labels().Set("invalid", invalid)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if v := data.Labels().Get("esgz"); v != "" {
			helpers.Anyhow("rmi", "-f", v)
		}
		if v := data.Labels().Get("invalid"); v != "" {
			helpers.Anyhow("rmi", "-f", v)
		}
		if reg != nil {
			reg.Cleanup(data, helpers)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		helpers.Ensure("--snapshotter=stargz", "pull", data.Labels().Get("invalid"))
		return helpers.Command("--snapshotter=stargz", "pull", data.Labels().Get("esgz"))
	}

	testCase.Expected = test.Expects(expect.ExitCodeGenericFail, []error{errors.New("existing images contain invalid ChainIDs")}, nil)

	testCase.Run(t)
}
