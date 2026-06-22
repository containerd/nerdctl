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

package main

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func TestAppNeedsRootlessParentMain(t *testing.T) {
	if !rootlessutil.IsRootlessParent() {
		t.Skip("test requires a rootless parent context")
	}

	app, err := newApp()
	assert.NilError(t, err)

	tests := []struct {
		name     string
		path     []string
		expected bool
	}{
		{
			name:     "root help path does not require reexec",
			path:     nil,
			expected: false,
		},
		{
			name:     "management command help path does not require reexec",
			path:     []string{"system"},
			expected: false,
		},
		{
			name:     "runtime command still requires reexec",
			path:     []string{"system", "info"},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := app
			args := []string{}
			if len(tc.path) > 0 {
				var findErr error
				cmd, args, findErr = app.Find(tc.path)
				assert.NilError(t, findErr)
			}
			assert.Equal(t, appNeedsRootlessParentMain(cmd, args), tc.expected)
		})
	}
}
