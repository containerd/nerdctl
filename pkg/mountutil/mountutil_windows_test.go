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

package mountutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	mocks "github.com/containerd/nerdctl/pkg/mountutil/mountutilmock"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestParseVolumeOptions(t *testing.T) {
	tests := []struct {
		vType    string
		src      string
		optsRaw  string
		wants    []string
		wantFail bool
	}{
		{
			vType:   "bind",
			src:     "dummy",
			optsRaw: "rw",
			wants:   []string{},
		},
		{
			vType:   "volume",
			src:     "dummy",
			optsRaw: "ro",
			wants:   []string{"ro"},
		},
		{
			vType:   "volume",
			src:     "dummy",
			optsRaw: "ro,undefined",
			wants:   []string{"ro"},
		},
		{
			vType:    "bind",
			src:      "dummy",
			optsRaw:  "ro,rw",
			wantFail: true,
		},
		{
			vType:    "volume",
			src:      "dummy",
			optsRaw:  "ro,ro",
			wantFail: true,
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join([]string{tt.vType, tt.src, tt.optsRaw}, "-"), func(t *testing.T) {
			opts, _, err := parseVolumeOptions(tt.vType, tt.src, tt.optsRaw)
			if err != nil {
				if tt.wantFail {
					return
				}
				t.Errorf("failed to parse option %q: %v", tt.optsRaw, err)
				return
			}
			assert.Equal(t, tt.wantFail, false)
			assert.Check(t, is.DeepEqual(tt.wants, opts))
		})
	}
}

func TestSplitRawSpec(t *testing.T) {
	tests := []struct {
		rawSpec string
		wants   []string
	}{
		// Absolute paths
		{
			rawSpec: `C:\TestVolume\Path:C:\TestVolume\Path:ro`,
			wants:   []string{`C:\TestVolume\Path`, `C:\TestVolume\Path`, "ro"},
		},
		{
			rawSpec: `C:\TestVolume\Path:C:\TestVolume\Path:ro,rw`,
			wants:   []string{`C:\TestVolume\Path`, `C:\TestVolume\Path`, "ro,rw"},
		},
		{
			rawSpec: `C:\TestVolume\Path:C:\TestVolume\Path:ro,undefined`,
			wants:   []string{`C:\TestVolume\Path`, `C:\TestVolume\Path`, "ro,undefined"},
		},
		{
			rawSpec: `C:\TestVolume\Path:C:\TestVolume\Path`,
			wants:   []string{`C:\TestVolume\Path`, `C:\TestVolume\Path`},
		},
		{
			rawSpec: `C:\TestVolume\Path`,
			wants:   []string{`C:\TestVolume\Path`},
		},
		{
			rawSpec: `C:\Test Volume\Path`, // space in path
			wants:   []string{`C:\Test Volume\Path`},
		},

		// Relative paths
		{
			rawSpec: `.\ContainerVolumes:C:\TestVolumes`,
			wants:   []string{`.\ContainerVolumes`, `C:\TestVolumes`},
		},
		{
			rawSpec: `.\ContainerVolumes:.\ContainerVolumes`,
			wants:   []string{`.\ContainerVolumes`, `.\ContainerVolumes`},
		},

		// Anonymous volumes
		{
			rawSpec: `.\ContainerVolumes`,
			wants:   []string{`.\ContainerVolumes`},
		},
		{
			rawSpec: `TestVolume`,
			wants:   []string{`TestVolume`},
		},
		{
			rawSpec: `:TestVolume`,
			wants:   []string{`TestVolume`},
		},

		// UNC paths
		{
			rawSpec: `\\?\UNC\server\share\path:.\ContainerVolumesto`,
			wants:   []string{`\\?\UNC\server\share\path`, `.\ContainerVolumesto`},
		},
		{
			rawSpec: `\\.\Volume{b75e2c83-0000-0000-0000-602f00000000}\Test`,
			wants:   []string{`\\.\Volume{b75e2c83-0000-0000-0000-602f00000000}\Test`},
		},

		// Named pipes
		{
			rawSpec: `\\.\pipe\containerd-containerd`,
			wants:   []string{`\\.\pipe\containerd-containerd`},
		},
		{
			rawSpec: `\\.\pipe\containerd-containerd:\\.\pipe\containerd-containerd`,
			wants:   []string{`\\.\pipe\containerd-containerd`, `\\.\pipe\containerd-containerd`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.rawSpec, func(t *testing.T) {
			actual, err := splitVolumeSpec(tt.rawSpec)
			if err != nil {
				t.Errorf("failed to split raw spec %q: %v", tt.rawSpec, err)
				return

			}
			assert.Check(t, is.DeepEqual(tt.wants, actual))
		})
	}
}

func TestSplitRawSpecInvalid(t *testing.T) {
	tests := []string{
		"",                                     // Empty string
		"   ",                                  // Empty string
		`.`,                                    // Invalid relative path
		`./`,                                   // Invalid relative path
		`../`,                                  // Invalid relative path
		`C:\`,                                  // Cannot mount root directory
		`~\TestVolume`,                         // Invalid relative path
		`..\TestVolume`,                        // Invalid relative path
		`ABC:\ContainerVolumes:C:\TestVolumes`, // Invalid drive letter
		`UNC\server\share\path`,                // Invalid path
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			_, err := splitVolumeSpec(path)
			if strings.TrimSpace(path) == "" {
				assert.Error(t, err, "invalid empty volume specification")
				return
			}
			if path == "." {
				assert.Error(t, err, "invalid volume specification: \".\"")
				return
			}
			assert.Error(t, err, fmt.Sprintf("invalid volume specification: '%s'", path))
		})
	}
}

func TestProcessFlagV(t *testing.T) {
	tests := []struct {
		rawSpec string
		wants   *Processed
		err     string
	}{
		// Bind volumes: absolute path
		{
			rawSpec: "C:/TestVolume/Path:C:/TestVolume/Path:ro",
			wants: &Processed{
				Type: "bind",
				Mount: specs.Mount{
					Type:        "",
					Destination: `C:\TestVolume\Path`,
					Source:      `C:\TestVolume\Path`,
					Options:     []string{"ro", "rbind"},
				}},
		},
		// Bind volumes: relative path
		{
			rawSpec: `.\TestVolume\Path:C:\TestVolume\Path`,
			wants: &Processed{
				Type: "bind",
				Mount: specs.Mount{
					Type:        "",
					Source:      "", // will not check source of relative paths
					Destination: `C:\TestVolume\Path`,
					Options:     []string{"rbind"},
				}},
		},
		// Named volumes
		{
			rawSpec: `TestVolume:C:\TestVolume\Path`,
			wants: &Processed{
				Type: "volume",
				Name: "TestVolume",
				Mount: specs.Mount{
					Type:        "",
					Source:      "", // source of anonymous volume is a generated path, so here will not check it.
					Destination: `C:\TestVolume\Path`,
					Options:     []string{"rbind"},
				}},
		},
		// Named pipes
		{
			rawSpec: `\\.\pipe\containerd-containerd:\\.\pipe\containerd-containerd`,
			wants: &Processed{
				Type: "npipe",
				Mount: specs.Mount{
					Type:        "",
					Source:      `\\.\pipe\containerd-containerd`,
					Destination: `\\.\pipe\containerd-containerd`,
					Options:     []string{"rbind"},
				}},
		},
		{
			rawSpec: `\\.\pipe\containerd-containerd:C:\TestVolume\Path`,
			err:     "invalid volume specification. named pipes can only be mapped to named pipes",
		},
		{
			rawSpec: `C:\TestVolume\Path:TestVolume`,
			err:     "expected an absolute path or a named pipe, got \"TestVolume\"",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVolumeStore := mocks.NewMockVolumeStore(ctrl)
	mockVolumeStore.
		EXPECT().
		Get(gomock.Any(), false).
		Return(&native.Volume{Name: "test_volume", Mountpoint: "C:\\test\\directory", Size: 1024}, nil).
		AnyTimes()
	mockVolumeStore.
		EXPECT().
		Create(gomock.Any(), nil).
		Return(&native.Volume{Name: "test_volume", Mountpoint: "C:\\test\\directory"}, nil).AnyTimes()

	mockOs := mocks.NewMockOs(ctrl)
	mockOs.EXPECT().Stat(gomock.Any()).Return(nil, nil).AnyTimes()

	for _, tt := range tests {
		t.Run(tt.rawSpec, func(t *testing.T) {
			processedVolSpec, err := ProcessFlagV(tt.rawSpec, mockVolumeStore, true)
			if err != nil {
				assert.Error(t, err, tt.err)
				return
			}

			assert.Equal(t, processedVolSpec.Type, tt.wants.Type)
			assert.Equal(t, processedVolSpec.Mount.Type, tt.wants.Mount.Type)
			assert.Equal(t, processedVolSpec.Mount.Destination, tt.wants.Mount.Destination)
			assert.DeepEqual(t, processedVolSpec.Mount.Options, tt.wants.Mount.Options)

			if tt.wants.Name != "" {
				assert.Equal(t, processedVolSpec.Name, tt.wants.Name)
			}
			if tt.wants.Mount.Source != "" {
				assert.Equal(t, processedVolSpec.Mount.Source, tt.wants.Mount.Source)
			}
		})
	}
}

func TestProcessFlagVAnonymousVolumes(t *testing.T) {
	tests := []struct {
		rawSpec string
		wants   *Processed
		err     string
	}{
		{
			rawSpec: `C:\TestVolume\Path`,
			wants: &Processed{
				Type: "volume",
				Mount: specs.Mount{
					Type:        "",
					Source:      "", // source of anonymous volume is a generated path, so here will not check it.
					Destination: `C:\TestVolume\Path`,
				}},
		},
		{
			rawSpec: `.\TestVolume\Path`,
			err:     "expected an absolute path",
		},
		{
			rawSpec: `TestVolume`,
			err:     "only directories can be mapped as anonymous volumes",
		},
		{
			rawSpec: `C:\TestVolume\Path::ro`,
			err:     "failed to split volume mount specification",
		},
		{
			rawSpec: `\\.\pipe\containerd-containerd`,
			err:     "only directories can be mapped as anonymous volumes",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVolumeStore := mocks.NewMockVolumeStore(ctrl)
	mockVolumeStore.
		EXPECT().
		Create(gomock.Any(), []string{}).
		Return(&native.Volume{Name: "test_volume", Mountpoint: "C:\\test\\directory"}, nil).
		AnyTimes()

	for _, tt := range tests {
		t.Run(tt.rawSpec, func(t *testing.T) {
			processedVolSpec, err := ProcessFlagV(tt.rawSpec, mockVolumeStore, true)
			if err != nil {
				assert.ErrorContains(t, err, tt.err)
				return
			}

			assert.Equal(t, processedVolSpec.Type, tt.wants.Type)
			assert.Assert(t, processedVolSpec.AnonymousVolume != "")
			assert.Equal(t, processedVolSpec.Mount.Type, tt.wants.Mount.Type)
			assert.Equal(t, processedVolSpec.Mount.Destination, tt.wants.Mount.Destination)

			if tt.wants.Mount.Source != "" {
				assert.Equal(t, processedVolSpec.Mount.Source, tt.wants.Mount.Source)
			}

			// for anonymous volumes, we want to make sure that the source is not the same as the destination
			assert.Assert(t, processedVolSpec.Mount.Source != processedVolSpec.Mount.Destination)
		})
	}
}
