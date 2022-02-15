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

package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/containerd/images"
	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

func TestBuild(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()

	base.Cmd("run", "--rm", imageName).AssertOutExactly("nerdctl-build-test-string\n")
}

func TestBuildWithCompression(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()
	const testFileName = "nerdctl-build-test"
	const testContent = "nerdctl"

	// Build an image
	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /`,
		testFileName)
	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)
	if err := os.WriteFile(filepath.Join(buildCtx, testFileName), []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}
	base.Cmd("build", "-t", imageName, "--compression=uncompressed", buildCtx).AssertOK()

	// Check layer compression
	layersNum := 0
	forEachLayerInImageTar(t, base, imageName, 1, func(i int, desc ocispec.Descriptor, blob []byte) {
		layersNum++
		assert.Equal(t, images.MediaTypeDockerSchema2Layer, desc.MediaType) // layer must be tar
		tr := tar.NewReader(bytes.NewReader(blob))                          // layer must be tar
		h, err := tr.Next()
		assert.NilError(t, err)
		assert.Equal(t, testFileName, strings.TrimPrefix(path.Clean("/"+h.Name), "/"))
		gotFile, err := io.ReadAll(tr)
		assert.NilError(t, err)
		assert.Equal(t, testContent, string(gotFile))
	})
	assert.Equal(t, 1, layersNum) // must contain only 1 layer
}

func forEachLayerInImageTar(t *testing.T, base *testutil.Base, imageName string, manifestsNum int, f func(int, ocispec.Descriptor, []byte)) {
	imgTar := filepath.Join(t.TempDir(), "img.tar")
	base.Cmd("save", "-o", imgTar, imageName).AssertOK()
	img, err := os.ReadFile(imgTar)
	assert.NilError(t, err)

	var index ocispec.Index
	blobs := make(map[digest.Digest][]byte)
	tr := tar.NewReader(bytes.NewReader(img))
	for {
		h, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			assert.NilError(t, err)
		}
		name := strings.TrimPrefix(path.Clean("/"+h.Name), "/")
		if name == "index.json" {
			assert.NilError(t, json.NewDecoder(tr).Decode(&index))
		} else if strings.HasPrefix(name, "blobs/sha256/") {
			d, err := digest.Parse("sha256:" + strings.TrimPrefix(name, "blobs/sha256/"))
			assert.NilError(t, err)
			c, err := io.ReadAll(tr)
			assert.NilError(t, err)
			blobs[d] = c
		}
	}
	assert.Equal(t, manifestsNum, len(index.Manifests))
	for _, m := range index.Manifests {
		manifestB, ok := blobs[m.Digest]
		if !ok {
			t.Fatalf("manifest blob %q not found: %+v", m.Digest, index)
		}
		var manifest ocispec.Manifest
		assert.NilError(t, json.Unmarshal(manifestB, &manifest))
		for i, l := range manifest.Layers {
			layerB, ok := blobs[l.Digest]
			if !ok {
				t.Fatalf("layer blob %q not found: index=%+v manifest=%+v", l.Digest, index, manifest)
			}
			f(i, l, layerB)
		}
	}
}

func TestBuildFromStdin(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-stdin"]
	`, testutil.CommonImage)

	base.Cmd("build", "-t", imageName, "-f", "-", ".").CmdOption(testutil.WithStdin(strings.NewReader(dockerfile))).AssertOutContains(imageName)
}

func TestBuildLocal(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const testFileName = "nerdctl-build-test"
	const testContent = "nerdctl"
	outputDir := t.TempDir()

	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /`,
		testFileName)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	if err := os.WriteFile(filepath.Join(buildCtx, testFileName), []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	testFilePath := filepath.Join(outputDir, testFileName)
	base.Cmd("build", "-o", fmt.Sprintf("type=local,dest=%s", outputDir), buildCtx).AssertOK()
	if _, err := os.Stat(testFilePath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(testFilePath)
	assert.NilError(t, err)
	assert.Equal(t, string(data), testContent)
}

func createBuildContext(dockerfile string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "nerdctl-build-test")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return "", err
	}
	return tmpDir, nil
}

func TestBuildWithIIDFile(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)
	fileName := filepath.Join(t.TempDir(), "id.txt")

	base.Cmd("build", "-t", imageName, buildCtx, "--iidfile", fileName).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName, "--iidfile", fileName).AssertOK()
	defer os.Remove(fileName)

	imageID, err := os.ReadFile(fileName)
	assert.NilError(t, err)

	base.Cmd("run", "--rm", string(imageID)).AssertOutExactly("nerdctl-build-test-string\n")
}

func TestBuildWithLabels(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)

	dockerfile := fmt.Sprintf(`FROM %s
LABEL name=nerdctl-build-test-label
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx, "--label", "label=test").AssertOK()
	defer base.Cmd("rmi", imageName).Run()

	base.Cmd("inspect", imageName, "--format", "{{json .Config.Labels }}").AssertOutExactly("{\"label\":\"test\",\"name\":\"nerdctl-build-test-label\"}\n")
}
