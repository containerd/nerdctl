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

package container

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func TestDetectGPUVendor(t *testing.T) {
	nvidiaSpec := `cdiVersion: "0.5.0"
kind: "nvidia.com/gpu"
devices:
  - name: "0"
    containerEdits:
      deviceNodes:
        - path: /dev/nvidia0
`
	amdSpec := `cdiVersion: "0.5.0"
kind: "amd.com/gpu"
devices:
  - name: "0"
    containerEdits:
      deviceNodes:
        - path: /dev/dri/renderD128
`
	unknownSpec := `cdiVersion: "0.5.0"
kind: "unknown.com/gpu"
devices:
  - name: "0"
    containerEdits:
      deviceNodes:
        - path: /dev/unknown0
`

	testCases := []struct {
		name           string
		specs          map[string]string
		expectedVendor string
	}{
		{
			name:           "no CDI specs returns empty",
			specs:          nil,
			expectedVendor: "",
		},
		{
			name:           "nvidia vendor detected",
			specs:          map[string]string{"nvidia.yaml": nvidiaSpec},
			expectedVendor: "nvidia.com",
		},
		{
			name:           "amd vendor detected",
			specs:          map[string]string{"amd.yaml": amdSpec},
			expectedVendor: "amd.com",
		},
		{
			name:           "detect first known vendor",
			specs:          map[string]string{"nvidia.yaml": nvidiaSpec, "amd.yaml": amdSpec},
			expectedVendor: "nvidia.com",
		},
		{
			name:           "unknown vendor ignored",
			specs:          map[string]string{"unknown.yaml": unknownSpec},
			expectedVendor: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for filename, content := range tc.specs {
				err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
				assert.NilError(t, err)
			}

			vendor := container.DetectGPUVendor([]string{tmpDir})
			assert.Equal(t, vendor, tc.expectedVendor)
		})
	}
}
