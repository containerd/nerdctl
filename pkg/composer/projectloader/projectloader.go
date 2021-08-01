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

package projectloader

import (
	"io/ioutil"
	"path/filepath"

	"github.com/compose-spec/compose-go/loader"
	compose "github.com/compose-spec/compose-go/types"
)

func Load(fileName, projectName string, envMap map[string]string) (*compose.Project, error) {
	b, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	wd, err := filepath.Abs(filepath.Dir(fileName))
	if err != nil {
		return nil, err
	}
	var files []compose.ConfigFile
	files = append(files, compose.ConfigFile{Filename: fileName, Content: b})
	return loader.Load(compose.ConfigDetails{
		WorkingDir:  wd,
		ConfigFiles: files,
		Environment: envMap,
	}, withProjectName(projectName))
}

func withProjectName(name string) func(*loader.Options) {
	return func(lOpts *loader.Options) {
		lOpts.Name = name
	}
}
