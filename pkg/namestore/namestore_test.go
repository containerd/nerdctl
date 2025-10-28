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

package namestore

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/store"
)

func TestNamestoreNew(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name      string
		namespace string
		wantErr   bool
		errChecks []error
	}{
		{
			name:      "empty namespace",
			namespace: "",
			wantErr:   true,
			errChecks: []error{ErrNameStore, store.ErrInvalidArgument},
		},
		{
			name:      "valid namespace",
			namespace: "testnamespace",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, err := New(tempDir, tt.namespace)
			if tt.wantErr {
				assert.Assert(t, err != nil, "New should return an error for %s", tt.name)
				for _, errCheck := range tt.errChecks {
					assert.ErrorIs(t, err, errCheck, "Error should contain %v for %s", errCheck, tt.name)
				}
			} else {
				assert.NilError(t, err, "New should succeed for %s", tt.name)
				assert.Assert(t, ns != nil, "New should return a non-nil NameStore for %s", tt.name)

				// Check that the directory is created in the correct path
				expectedDir := filepath.Join(tempDir, "names", tt.namespace)
				_, err = os.Stat(expectedDir)
				assert.NilError(t, err, "Directory should be created at the correct path for %s", tt.name)
			}
		})
	}
}
