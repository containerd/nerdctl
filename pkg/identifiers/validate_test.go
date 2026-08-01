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

package identifiers

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"
)

func TestValidateDockerCompat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "two alphanumeric", input: "ab"},
		{name: "digits", input: "12"},
		{name: "with underscore", input: "my_container"},
		{name: "with dash and dot", input: "my-container.1"},
		{name: "mixed separators", input: "A.b_c-2"},
		{name: "empty", input: "", wantErr: "identifier must not be empty"},
		{name: "single character", input: "a", wantErr: "must match pattern"},
		{name: "leading underscore", input: "_ab", wantErr: "must match pattern"},
		{name: "leading dash", input: "-ab", wantErr: "must match pattern"},
		{name: "leading dot", input: ".ab", wantErr: "must match pattern"},
		{name: "contains space", input: "a b", wantErr: "must match pattern"},
		{name: "contains slash", input: "a/b", wantErr: "must match pattern"},
		{name: "contains colon", input: "a:b", wantErr: "must match pattern"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDockerCompat(tc.input)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.ErrorIs(t, err, errdefs.ErrInvalidArgument)
				return
			}
			assert.NilError(t, err)
		})
	}
}
