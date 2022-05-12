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

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
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
