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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"

	"gotest.tools/v3/assert"
)

func TestSave(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	archiveTarPath := filepath.Join(t.TempDir(), "a.tar")
	base.Cmd("save", "-o", archiveTarPath, testutil.AlpineImage).AssertOK()
	rootfsPath := filepath.Join(t.TempDir(), "rootfs")
	err := extractDockerArchive(archiveTarPath, rootfsPath)
	assert.NilError(t, err)
	etcOSReleasePath := filepath.Join(rootfsPath, "/etc/os-release")
	etcOSReleaseBytes, err := os.ReadFile(etcOSReleasePath)
	assert.NilError(t, err)
	etcOSRelease := string(etcOSReleaseBytes)
	t.Logf("read %q, extracted from %q", etcOSReleasePath, testutil.AlpineImage)
	t.Log(etcOSRelease)
	assert.Assert(t, strings.Contains(etcOSRelease, "Alpine"))
}

func TestSaveById(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	inspect := base.InspectImage(testutil.AlpineImage)
	var id string
	if testutil.GetTarget() == testutil.Docker {
		id = inspect.ID
	} else {
		id = strings.Split(inspect.RepoDigests[0], ":")[1]
	}
	archiveTarPath := filepath.Join(t.TempDir(), "id.tar")
	base.Cmd("save", "-o", archiveTarPath, id).AssertOK()
	base.Cmd("rmi", "-f", testutil.AlpineImage).AssertOK()
	base.Cmd("load", "-i", archiveTarPath).AssertOK()
	base.Cmd("run", "--rm", id, "date").AssertOK()
}

func extractDockerArchive(archiveTarPath, rootfsPath string) error {
	if err := os.MkdirAll(rootfsPath, 0755); err != nil {
		return err
	}
	workDir, err := os.MkdirTemp("", "extract-docker-archive")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	if err := extractTarFile(workDir, archiveTarPath); err != nil {
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
		if err := extractTarFile(rootfsPath, layerTarPath); err != nil {
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

func extractTarFile(dirPath, tarFilePath string) error {
	cmd := exec.Command("tar", "Cxf", dirPath, tarFilePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run %v: %q: %w", cmd.Args, string(out), err)
	}
	return nil
}
