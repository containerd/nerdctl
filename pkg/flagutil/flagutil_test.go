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

package flagutil

import (
	"fmt"
	"sort"
	"testing"

	"gotest.tools/v3/assert"
)

func TestReplaceOrAppendEnvValues(t *testing.T) {

	tests := []struct {
		defaults  []string
		overrides []string
		expected  []string
	}{
		// override defaults
		{
			defaults:  []string{"A=default", "B=default"},
			overrides: []string{"A=override", "C=override"},
			expected:  []string{"A=override", "B=default", "C=override"},
		},
		// empty defaults
		{
			defaults:  []string{"A=default", "B=default"},
			overrides: []string{"A=override", "B="},
			expected:  []string{"A=override", "B="},
		},
		// remove defaults
		{
			defaults:  []string{"A=default", "B=default"},
			overrides: []string{"A=override", "B"},
			expected:  []string{"A=override"},
		},
	}

	comparator := func(s1, s2 []string) bool {
		if len(s1) != len(s2) {
			return false
		}
		sort.Slice(s1, func(i, j int) bool {
			return s1[i] < s1[j]
		})
		sort.Slice(s2, func(i, j int) bool {
			return s2[i] < s2[j]
		})
		for i, v := range s1 {
			if v != s2[i] {
				return false
			}
		}
		return true
	}
	for _, tt := range tests {
		actual := ReplaceOrAppendEnvValues(tt.defaults, tt.overrides)
		assert.Assert(t, comparator(actual, tt.expected), fmt.Sprintf("expected: %s, actual: %s", tt.expected, actual))
	}
}
