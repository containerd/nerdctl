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

package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/loader"
	compose "github.com/compose-spec/compose-go/v2/types"

	"github.com/containerd/nerdctl/mod/tigron/tig"
)

type ComposeDir struct {
	t            tig.T
	dir          string
	yamlBasePath string
}

func (cd *ComposeDir) WriteFile(name, content string) {
	if err := os.WriteFile(filepath.Join(cd.dir, name), []byte(content), 0644); err != nil {
		cd.t.Log(fmt.Sprintf("Failed to create file %s", err))
		cd.t.FailNow()
	}
}

func (cd *ComposeDir) YAMLFullPath() string {
	return filepath.Join(cd.dir, cd.yamlBasePath)
}

func (cd *ComposeDir) Dir() string {
	return cd.dir
}

func (cd *ComposeDir) ProjectName() string {
	return filepath.Base(cd.dir)
}

func (cd *ComposeDir) CleanUp() {
	os.RemoveAll(cd.dir)
}

func NewComposeDir(t tig.T, dockerComposeYAML string) *ComposeDir {
	tmpDir, err := os.MkdirTemp("", "nerdctl-compose-test")
	if err != nil {
		t.Log(fmt.Sprintf("Failed to create temp dir: %s", err))
		t.FailNow()
	}
	cd := &ComposeDir{
		t:            t,
		dir:          tmpDir,
		yamlBasePath: "docker-compose.yaml",
	}
	cd.WriteFile(cd.yamlBasePath, dockerComposeYAML)
	return cd
}

// Load is used only for unit testing.
func LoadProject(fileName, projectName string, envMap map[string]string) (*compose.Project, error) {
	if envMap == nil {
		envMap = make(map[string]string)
	}
	b, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	wd, err := filepath.Abs(filepath.Dir(fileName))
	if err != nil {
		return nil, err
	}
	var files []compose.ConfigFile
	files = append(files, compose.ConfigFile{Filename: fileName, Content: b})
	return loader.LoadWithContext(context.TODO(), compose.ConfigDetails{
		WorkingDir:  wd,
		ConfigFiles: files,
		Environment: envMap,
	}, withProjectName(projectName))
}

func withProjectName(name string) func(*loader.Options) {
	return func(lOpts *loader.Options) {
		lOpts.SetProjectName(name, true)
	}
}
