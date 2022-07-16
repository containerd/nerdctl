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

/*
   Portions from https://github.com/docker/cli/blob/v20.10.9/cli/command/image/build/context.go
   Copyright (C) Docker authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/docker/cli/blob/v20.10.9/NOTICE
*/

package buildkitutil

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestBuildKitFile(t *testing.T) {
	var tmp = t.TempDir()
	var wd, err = os.Getwd()
	assert.NilError(t, err)
	err = os.Chdir(tmp)
	assert.NilError(t, err)
	defer os.Chdir(wd)
	type args struct {
		dir       string
		inputfile string
	}
	tests := []struct {
		name       string
		args       args
		prepare    func(t *testing.T) error
		wantAbsDir string
		wantFile   string
		wantErr    bool
	}{
		{
			name: "only Dockerfile is present",
			prepare: func(t *testing.T) error {
				return os.WriteFile(filepath.Join(tmp, DefaultDockerfileName), []byte{}, 0644)
			},
			args:       args{".", ""},
			wantAbsDir: tmp,
			wantFile:   DefaultDockerfileName,
			wantErr:    false,
		},
		{
			name: "only Containerfile is present",
			prepare: func(t *testing.T) error {
				return os.WriteFile(filepath.Join(tmp, "Containerfile"), []byte{}, 0644)
			},
			args:       args{".", ""},
			wantAbsDir: tmp,
			wantFile:   ContainerfileName,
			wantErr:    false,
		},
		{
			name: "both Dockerfile and Containerfile are present",
			prepare: func(t *testing.T) error {
				var err = os.WriteFile(filepath.Join(tmp, "Dockerfile"), []byte{}, 0644)
				if err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(tmp, "Containerfile"), []byte{}, 0644)
			},
			args:       args{".", ""},
			wantAbsDir: tmp,
			wantFile:   DefaultDockerfileName,
			wantErr:    false,
		},
		{
			name: "Dockerfile and Containerfile have different contents",
			prepare: func(t *testing.T) error {
				var err = os.WriteFile(filepath.Join(tmp, "Dockerfile"), []byte{'d'}, 0644)
				if err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(tmp, "Containerfile"), []byte{'c'}, 0644)
			},
			args:       args{".", ""},
			wantAbsDir: tmp,
			wantFile:   DefaultDockerfileName,
			wantErr:    false,
		},
		{
			name: "Custom file is specfied",
			prepare: func(t *testing.T) error {
				return os.WriteFile(filepath.Join(tmp, "CustomFile"), []byte{}, 0644)
			},
			args:       args{".", "CustomFile"},
			wantAbsDir: tmp,
			wantFile:   "CustomFile",
			wantErr:    false,
		},
		{
			name: "Absolute path is specified along with custom file",
			prepare: func(t *testing.T) error {
				return os.WriteFile(filepath.Join(tmp, "CustomFile"), []byte{}, 0644)
			},
			args:       args{tmp, "CustomFile"},
			wantAbsDir: tmp,
			wantFile:   "CustomFile",
			wantErr:    false,
		},
		{
			name: "Absolute path is specified along with Docker file",
			prepare: func(t *testing.T) error {
				return os.WriteFile(filepath.Join(tmp, "Dockerfile"), []byte{}, 0644)
			},
			args:       args{tmp, "."},
			wantAbsDir: tmp,
			wantFile:   DefaultDockerfileName,
			wantErr:    false,
		},
		{
			name: "Absolute path is specified with Container file in the path",
			prepare: func(t *testing.T) error {
				return os.WriteFile(filepath.Join(tmp, ContainerfileName), []byte{}, 0644)
			},
			args:       args{tmp, "."},
			wantAbsDir: tmp,
			wantFile:   ContainerfileName,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.prepare(t)
			gotAbsDir, gotFile, err := BuildKitFile(tt.args.dir, tt.args.inputfile)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildKitFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotAbsDir != tt.wantAbsDir {
				t.Errorf("BuildKitFile() gotAbsDir = %v, want %v", gotAbsDir, tt.wantAbsDir)
			}
			if gotFile != tt.wantFile {
				t.Errorf("BuildKitFile() gotFile = %v, want %v", gotFile, tt.wantFile)
			}

			entry, err := os.ReadDir(tmp)
			assert.NilError(t, err)
			for _, f := range entry {
				err = os.Remove(f.Name())
				assert.NilError(t, err)
			}
		})
	}
}
