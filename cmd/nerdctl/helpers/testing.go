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

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func CreateBuildContext(t *testing.T, dockerfile string) string {
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644)
	assert.NilError(t, err)
	return tmpDir
}

func RmiAll(base *testutil.Base) {
	base.T.Logf("Pruning images")
	imageIDs := base.Cmd("images", "--no-trunc", "-a", "-q").OutLines()
	// remove empty output line at the end
	imageIDs = imageIDs[:len(imageIDs)-1]
	// use `Run` on purpose (same below) because `rmi all` may fail on individual
	// image id that has an expected running container (e.g. a registry)
	base.Cmd(append([]string{"rmi", "-f"}, imageIDs...)...).Run()

	base.T.Logf("Pruning build caches")
	if _, err := buildkitutil.GetBuildkitHost(testutil.Namespace); err == nil {
		base.Cmd("builder", "prune", "--force").AssertOK()
	}

	// For BuildKit >= 0.11, pruning cache isn't enough to remove manifest blobs that are referred by build history blobs
	// https://github.com/containerd/nerdctl/pull/1833
	if base.Target == testutil.Nerdctl {
		base.T.Logf("Pruning all content blobs")
		addr := base.ContainerdAddress()
		client, err := containerd.New(addr, containerd.WithDefaultNamespace(testutil.Namespace))
		assert.NilError(base.T, err)
		cs := client.ContentStore()
		ctx := context.TODO()
		wf := func(info content.Info) error {
			base.T.Logf("Pruning blob %+v", info)
			if err := cs.Delete(ctx, info.Digest); err != nil {
				base.T.Log(err)
			}
			return nil
		}
		if err := cs.Walk(ctx, wf); err != nil {
			base.T.Log(err)
		}

		base.T.Logf("Pruning all images (again?)")
		imageIDs = base.Cmd("images", "--no-trunc", "-a", "-q").OutLines()
		base.T.Logf("pruning following images: %+v", imageIDs)
		base.Cmd(append([]string{"rmi", "-f"}, imageIDs...)...).Run()
	}
}

func ExtractDockerArchive(archiveTarPath, rootfsPath string) error {
	if err := os.MkdirAll(rootfsPath, 0755); err != nil {
		return err
	}
	workDir, err := os.MkdirTemp("", "extract-docker-archive")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	if err := ExtractTarFile(workDir, archiveTarPath); err != nil {
		return err
	}
	manifestJSONPath := filepath.Join(workDir, "manifest.json")
	manifestJSONBytes, err := os.ReadFile(manifestJSONPath)
	if err != nil {
		return err
	}
	var mani DockerArchiveManifestJSON
	if err := json.Unmarshal(manifestJSONBytes, &mani); err != nil {
		return err
	}
	if len(mani) > 1 {
		return fmt.Errorf("multi-image archive cannot be extracted: contains %d images", len(mani))
	}
	if len(mani) < 1 {
		return errors.New("invalid archive")
	}
	ent := mani[0]
	for _, l := range ent.Layers {
		layerTarPath := filepath.Join(workDir, l)
		if err := ExtractTarFile(rootfsPath, layerTarPath); err != nil {
			return err
		}
	}
	return nil
}

type DockerArchiveManifestJSON []DockerArchiveManifestJSONEntry

type DockerArchiveManifestJSONEntry struct {
	Config   string
	RepoTags []string
	Layers   []string
}

func ExtractTarFile(dirPath, tarFilePath string) error {
	cmd := exec.Command("tar", "Cxf", dirPath, tarFilePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run %v: %q: %w", cmd.Args, string(out), err)
	}
	return nil
}
