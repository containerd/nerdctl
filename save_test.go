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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
)

func TestSave(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	tmpDir, err := ioutil.TempDir("", "test-save")
	assert.NilError(t, err)
	defer os.RemoveAll(tmpDir)
	archiveTarPath := filepath.Join(tmpDir, "a.tar")
	base.Cmd("save", "-o", archiveTarPath, testutil.AlpineImage).AssertOK()
	rootfsPath := filepath.Join(tmpDir, "rootfs")
	err = extractDockerArchive(archiveTarPath, rootfsPath)
	assert.NilError(t, err)
	etcOSReleasePath := filepath.Join(rootfsPath, "/etc/os-release")
	etcOSReleaseBytes, err := ioutil.ReadFile(etcOSReleasePath)
	assert.NilError(t, err)
	etcOSRelease := string(etcOSReleaseBytes)
	t.Logf("read %q, extracted from %q", etcOSReleasePath, testutil.AlpineImage)
	t.Log(etcOSRelease)
	assert.Assert(t, strings.Contains(etcOSRelease, "Alpine"))
}

func extractDockerArchive(archiveTarPath, rootfsPath string) error {
	if err := os.MkdirAll(rootfsPath, 0755); err != nil {
		return err
	}
	workDir, err := ioutil.TempDir("", "extract-docker-archive")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	if err := extractTarFile(workDir, archiveTarPath); err != nil {
		return err
	}
	manifestJSONPath := filepath.Join(workDir, "manifest.json")
	manifestJSONBytes, err := ioutil.ReadFile(manifestJSONPath)
	if err != nil {
		return err
	}
	var mani DockerArchiveManifestJSON
	if err := json.Unmarshal(manifestJSONBytes, &mani); err != nil {
		return err
	}
	if len(mani) > 1 {
		return errors.Errorf("multi-image archive cannot be extracted: contains %d images", len(mani))
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
		return errors.Wrapf(err, "failed to run %v: %q",
			cmd.Args,
			string(out))
	}
	return nil
}
