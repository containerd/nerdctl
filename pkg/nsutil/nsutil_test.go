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

package nsutil_test

import (
	"testing"

	"github.com/containerd/nerdctl/pkg/nsutil"
	"gotest.tools/v3/assert"
)

func TestValidateNamespaceName(t *testing.T) {
	testCases := []struct {
		inputs    []string
		errSubstr string
	}{
		{
			[]string{"test", "test-hyphen", ".start.dot", "mid.dot", "end.dot."},
			"",
		},
		{
			[]string{".", "..", "~"},
			"namespace name cannot be special path alias",
		},
		{
			[]string{"$$", "a$VARiable", "a%VAR%iable", "\\.", "\\%", "\\$"},
			"namespace name cannot contain any special characters",
		},
		{
			[]string{"/start", "mid/dle", "end/", "\\start", "mid\\dle", "end\\"},
			"namespace name cannot contain any special characters",
		},
	}

	for _, tc := range testCases {
		for _, input := range tc.inputs {
			err := nsutil.ValidateNamespaceName(input)
			if tc.errSubstr == "" {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.errSubstr)
			}
		}
	}
}
