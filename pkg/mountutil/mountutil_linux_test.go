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
	"context"
	"strings"
	"testing"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	mocks "github.com/containerd/nerdctl/v2/pkg/mountutil/mountutilmock"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

// TestParseVolumeOptions tests volume options are parsed as expected.
func TestParseVolumeOptions(t *testing.T) {
	tests := []struct {
		name                     string
		vType                    string
		src                      string
		optsRaw                  string
		srcOptional              []string
		initialRootfsPropagation string
		wants                    []string
		wantRootfsPropagation    string
		wantFail                 bool
	}{
		{
			name:    "unknown option is ignored (with warning)",
			vType:   "volume",
			src:     "dummy",
			optsRaw: "ro,undefined",
			wants:   []string{"ro"},
		},

		// tests for rw/ro flags
		{
			name:    "read write",
			vType:   "bind",
			src:     "dummy",
			optsRaw: "rw",
			wants:   []string{"rprivate"},
		},
		{
			name:    "read only",
			vType:   "volume",
			src:     "dummy",
			optsRaw: "ro",
			wants:   []string{"ro"},
		},
		{
			name:     "duplicated flags are not allowed",
			vType:    "bind",
			src:      "dummy",
			optsRaw:  "ro,rw",
			wantFail: true,
		},
		{
			name:     "duplicated flags (ro/ro) are not allowed",
			vType:    "volume",
			src:      "dummy",
			optsRaw:  "ro,ro",
			wantFail: true,
		},

		// tests for propagation flags
		{
			name:     "volume doesn't accept propagation option",
			vType:    "volume",
			src:      "dummy",
			optsRaw:  "private",
			wantFail: true,
		},
		{
			name:     "duplicated propagation option is not allowed",
			vType:    "bind",
			src:      "dummy",
			optsRaw:  "private,shared",
			wantFail: true,
		},
		{
			name:  "default propagation type is rprivate",
			vType: "bind",
			src:   "dummy",
			wants: []string{"rprivate"},
		},
		{
			name:    "make bind private",
			vType:   "bind",
			src:     "dummy",
			optsRaw: "ro,private",
			wants:   []string{"ro", "private"},
		},
		{
			name:    "make bind nonrecursive",
			vType:   "bind",
			src:     "dummy",
			optsRaw: "bind",
			wants:   []string{"bind", "rprivate"},
		},
		{
			name:                  "make bind shared",
			vType:                 "bind",
			src:                   "dummy",
			optsRaw:               "ro,rshared",
			srcOptional:           []string{"shared:xxx"},
			wantRootfsPropagation: "shared",
			wants:                 []string{"ro", "rshared"},
		},
		{
			name:                     "make bind shared (unchange RootfsPropagation)",
			vType:                    "bind",
			src:                      "dummy",
			optsRaw:                  "ro,rshared",
			srcOptional:              []string{"shared:xxx"},
			initialRootfsPropagation: "rshared",
			wantRootfsPropagation:    "rshared",
			wants:                    []string{"ro", "rshared"},
		},
		{
			name:        "shared propagation is not allowed if the src is not shared",
			vType:       "bind",
			src:         "dummy",
			optsRaw:     "ro,shared",
			srcOptional: nil,
			wantFail:    true,
		},
		{
			name:                  "make bind slave",
			vType:                 "bind",
			src:                   "dummy",
			optsRaw:               "ro,slave",
			srcOptional:           []string{"master:xxx"},
			wantRootfsPropagation: "rslave",
			wants:                 []string{"ro", "slave"},
		},
		{
			name:                     "make bind slave (unchange RootfsPropagation)",
			vType:                    "bind",
			src:                      "dummy",
			optsRaw:                  "ro,slave",
			srcOptional:              []string{"master:xxx"},
			initialRootfsPropagation: "shared",
			wantRootfsPropagation:    "shared",
			wants:                    []string{"ro", "slave"},
		},
		{
			name:        "slave propagation is not allowed if the src is not slave",
			vType:       "bind",
			src:         "dummy",
			optsRaw:     "ro,slave",
			srcOptional: nil,
			wantFail:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, specOpts, err := parseVolumeOptionsWithMountInfo(tt.vType, tt.src, tt.optsRaw, func(string) (mount.Info, error) {
				return mount.Info{
					Mountpoint: tt.src,
					Optional:   strings.Join(tt.srcOptional, " "),
				}, nil
			})
			if err != nil {
				if tt.wantFail {
					return
				}
				t.Errorf("failed to parse option %q: %v", tt.optsRaw, err)
				return
			}
			s := oci.Spec{Linux: &specs.Linux{RootfsPropagation: tt.initialRootfsPropagation}}
			for _, o := range specOpts {
				assert.NilError(t, o(context.Background(), nil, nil, &s))
			}
			assert.Equal(t, tt.wantRootfsPropagation, s.Linux.RootfsPropagation)
			assert.Equal(t, tt.wantFail, false)
			assert.Check(t, is.DeepEqual(tt.wants, opts))
		})
	}
}

func TestProcessTmpfs(t *testing.T) {
	testCases := map[string][]string{
		"/tmp":               {"noexec", "nosuid", "nodev"},
		"/tmp:size=64m,exec": {"nosuid", "nodev", "size=64m", "exec"},
	}
	for k, expected := range testCases {
		x, err := ProcessFlagTmpfs(k)
		assert.NilError(t, err)
		assert.DeepEqual(t, expected, x.Mount.Options)
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
			rawSpec: "/mnt/foo:/mnt/foo:ro",
			wants: &Processed{
				Type: "bind",
				Mount: specs.Mount{
					Type:        "none",
					Destination: `/mnt/foo`,
					Source:      `/mnt/foo`,
					Options:     []string{"ro", "rprivate", "rbind"},
				}},
		},
		// Bind volumes: relative path
		{
			rawSpec: `./TestVolume/Path:/mnt/foo`,
			wants: &Processed{
				Type: "bind",
				Mount: specs.Mount{
					Type:        "none",
					Source:      "", // will not check source of relative paths
					Destination: `/mnt/foo`,
					Options:     []string{"rbind"},
				}},
		},
		// Named volumes
		{
			rawSpec: `TestVolume:/mnt/foo`,
			wants: &Processed{
				Type: "volume",
				Name: "TestVolume",
				Mount: specs.Mount{
					Type:        "none",
					Source:      "", // source of anonymous volume is a generated path, so here will not check it.
					Destination: `/mnt/foo`,
					Options:     []string{"rbind"},
				}},
		},
		{
			rawSpec: `/mnt/foo:TestVolume`,
			err:     "expected an absolute path, got \"TestVolume\"",
		},
		{
			rawSpec: `/mnt/foo:./foo`,
			err:     "expected an absolute path, got \"./foo\"",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVolumeStore := mocks.NewMockVolumeStore(ctrl)
	mockVolumeStore.
		EXPECT().
		Get(gomock.Any(), false).
		Return(&native.Volume{Name: "test_volume", Mountpoint: "/test/volume", Size: 1024}, nil).
		AnyTimes()
	mockVolumeStore.
		EXPECT().
		Create(gomock.Any(), nil).
		Return(&native.Volume{Name: "test_volume", Mountpoint: "/test/volume"}, nil).AnyTimes()

	mockOs := mocks.NewMockOs(ctrl)
	mockOs.EXPECT().Stat(gomock.Any()).Return(nil, nil).AnyTimes()

	for _, tt := range tests {
		t.Run(tt.rawSpec, func(t *testing.T) {
			processedVolSpec, err := ProcessFlagV(tt.rawSpec, mockVolumeStore, false)
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
			rawSpec: `/mnt/foo`,
			wants: &Processed{
				Type: "volume",
				Mount: specs.Mount{
					Type:        "none",
					Source:      "", // source of anonymous volume is a generated path, so here will not check it.
					Destination: `/mnt/foo`,
				}},
		},
		{
			rawSpec: `./TestVolume/Path`,
			wants: &Processed{
				Type: "volume",
				Mount: specs.Mount{
					Type:        "none",
					Source:      "",                // source of anonymous volume is a generated path, so here will not check it.
					Destination: `TestVolume/Path`, // cleanpath() removes the leading "./". Since we are mocking the os.Stat() call, this is fine.
				}},
		},
		{
			rawSpec: "TestVolume",
			wants: &Processed{
				Type: "volume",
				Mount: specs.Mount{
					Type:        "none",
					Source:      "", // source of anonymous volume is a generated path, so here will not check it.
					Destination: "TestVolume",
				}},
		},
		{
			rawSpec: `/mnt/foo::ro`,
			err:     "expected an absolute path, got \"\"",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVolumeStore := mocks.NewMockVolumeStore(ctrl)
	mockVolumeStore.
		EXPECT().
		Create(gomock.Any(), []string{}).
		Return(&native.Volume{Name: "test_volume", Mountpoint: "/test/volume"}, nil).
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
