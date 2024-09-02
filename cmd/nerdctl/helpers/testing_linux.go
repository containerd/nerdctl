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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

func ComposeUp(t *testing.T, base *testutil.Base, dockerComposeYAML string, opts ...string) {
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd(append(append([]string{"-f", comp.YAMLFullPath()}, opts...), "up", "-d")...).AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	base.Cmd("volume", "inspect", fmt.Sprintf("%s_db", projectName)).AssertOK()
	base.Cmd("network", "inspect", fmt.Sprintf("%s_default", projectName)).AssertOK()

	checkWordpress := func() error {
		resp, err := nettestutil.HTTPGet("http://127.0.0.1:8080", 10, false)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(respBody), testutil.WordpressIndexHTMLSnippet) {
			t.Logf("respBody=%q", respBody)
			return fmt.Errorf("respBody does not contain %q", testutil.WordpressIndexHTMLSnippet)
		}
		return nil
	}

	var wordpressWorking bool
	for i := 0; i < 30; i++ {
		t.Logf("(retry %d)", i)
		err := checkWordpress()
		if err == nil {
			wordpressWorking = true
			break
		}
		// NOTE: "<h1>Error establishing a database connection</h1>" is expected for the first few iterations
		t.Log(err)
		time.Sleep(3 * time.Second)
	}

	if !wordpressWorking {
		t.Fatal("wordpress is not working")
	}
	t.Log("wordpress seems functional")

	base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()
	base.Cmd("volume", "inspect", fmt.Sprintf("%s_db", projectName)).AssertFail()
	base.Cmd("network", "inspect", fmt.Sprintf("%s_default", projectName)).AssertFail()
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
