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

package volume

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestMountedVolumes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		mountsJSON string
		expected   []string
	}{
		{
			name:       "no mounts",
			mountsJSON: "",
		},
		{
			name: "a named volume and a bind",
			mountsJSON: `[{"Type":"volume","Name":"data","Destination":"/data"},` +
				`{"Type":"bind","Source":"/host","Destination":"/host"}]`,
			expected: []string{"data"},
		},
		{
			name: "the same volume at two paths counts once",
			mountsJSON: `[{"Type":"volume","Name":"data","Destination":"/data"},` +
				`{"Type":"volume","Name":"data","Destination":"/backup"}]`,
			expected: []string{"data"},
		},
		{
			name: "two volumes",
			mountsJSON: `[{"Type":"volume","Name":"data","Destination":"/data"},` +
				`{"Type":"volume","Name":"logs","Destination":"/logs"}]`,
			expected: []string{"data", "logs"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			names, err := mountedVolumes(tc.mountsJSON)
			assert.NilError(t, err)
			assert.Equal(t, len(names), len(tc.expected))
			for _, name := range tc.expected {
				_, ok := names[name]
				assert.Assert(t, ok, name)
			}
		})
	}
}

func TestMountedVolumesInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := mountedVolumes(`[{"Type":"volume"`)
	assert.Assert(t, err != nil)
}
