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

package infoutil

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
)

func TestParseBuildctlVersion(t *testing.T) {
	testCases := map[string]*dockercompat.ComponentVersion{
		"buildctl github.com/moby/buildkit v0.10.3 c8d25d9a103b70dc300a4fd55e7e576472284e31": {
			Name:    "buildctl",
			Version: "v0.10.3",
			Details: map[string]string{
				"GitCommit": "c8d25d9a103b70dc300a4fd55e7e576472284e31",
			},
		},
		"buildctl github.com/moby/buildkit v0.10.0-380-g874eef9b 874eef9b70dbaf4f074d2bc8f4dc64237f8e83a0": {
			Name:    "buildctl",
			Version: "v0.10.0-380-g874eef9b",
			Details: map[string]string{
				"GitCommit": "874eef9b70dbaf4f074d2bc8f4dc64237f8e83a0",
			},
		},
		"buildctl github.com/moby/buildkit 0.0.0+unknown": {
			Name:    "buildctl",
			Version: "0.0.0+unknown",
		},
		"foo bar baz": nil,
	}

	for s, expected := range testCases {
		got, err := parseBuildctlVersion([]byte(s))
		if expected != nil {
			assert.NilError(t, err)
			assert.DeepEqual(t, expected, got)
		} else {
			assert.Assert(t, err != nil)
		}
	}
}

func TestParseRuncVersion(t *testing.T) {
	testCases := map[string]*dockercompat.ComponentVersion{
		`runc version 1.1.2
commit: v1.1.2-0-ga916309f
spec: 1.0.2-dev
go: go1.18.3
libseccomp: 2.5.1`: {
			Name:    "runc",
			Version: "1.1.2",
			Details: map[string]string{
				"GitCommit": "v1.1.2-0-ga916309f",
			},
		},
	}

	for s, expected := range testCases {
		got, err := parseRuncVersion([]byte(s))
		if expected != nil {
			assert.NilError(t, err)
			assert.DeepEqual(t, expected, got)
		} else {
			assert.Assert(t, err != nil)
		}
	}
}
