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

package login

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestSchemeWarning(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		wantWarning bool
		wantHint    bool
	}{
		{
			name:    "empty address",
			address: "",
		},
		{
			name:    "no scheme",
			address: "index.docker.io",
		},
		{
			name:    "hostname with port but no scheme",
			address: "localhost:5000",
		},
		{
			name:        "https scheme",
			address:     "https://index.docker.io/v1/",
			wantWarning: true,
		},
		{
			name:        "https scheme with port",
			address:     "https://localhost:5000",
			wantWarning: true,
		},
		{
			name:        "http scheme",
			address:     "http://localhost:5000",
			wantWarning: true,
			wantHint:    true,
		},
		{
			// Unsupported schemes are rejected with an explicit error by
			// dockerconfigresolver.Parse, so no warning is needed.
			name:    "unsupported scheme",
			address: "oci://registry.example.com",
		},
		{
			// The experimental scheme is meaningful and not ignored, so no warning.
			name:    "experimental scheme",
			address: "nerdctl-experimental://index.docker.io/v1/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			warning := schemeWarning(tc.address)
			assert.Equal(t, warning != "", tc.wantWarning)
			if tc.wantWarning {
				assert.Assert(t, strings.Contains(warning, "ignored"))
			}
			assert.Equal(t, strings.Contains(warning, "--insecure-registry"), tc.wantHint)
		})
	}
}
