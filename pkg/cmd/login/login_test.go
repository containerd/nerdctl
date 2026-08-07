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
	"context"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
)

func TestLoginAuthCredsAcceptsEquivalentHosts(t *testing.T) {
	tests := []struct {
		name    string
		address string
		acArg   string
		wantErr bool
	}{
		{
			name:    "exact host with standard port",
			address: "harbor.example.io",
			acArg:   "harbor.example.io:443",
		},
		{
			// https://github.com/containerd/nerdctl/issues/3992
			name:    "host without default port",
			address: "harbor.example.io",
			acArg:   "harbor.example.io",
		},
		{
			// https://github.com/containerd/nerdctl/issues/3245
			name:    "docker.io alias without port",
			address: "docker.io",
			acArg:   "registry-1.docker.io",
		},
		{
			name:    "docker.io alias with port",
			address: "docker.io",
			acArg:   "registry-1.docker.io:443",
		},
		{
			name:    "mismatched host",
			address: "harbor.example.io",
			acArg:   "evil.example.io",
			wantErr: true,
		},
		{
			name:    "explicit non-standard port not dropped",
			address: "harbor.example.io:8443",
			acArg:   "harbor.example.io",
			wantErr: true,
		},
		{
			name:    "different explicit port",
			address: "harbor.example.io",
			acArg:   "harbor.example.io:8443",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registryURL, err := dockerconfigresolver.Parse(tt.address)
			assert.NilError(t, err)
			credentials := &dockerconfigresolver.Credentials{Username: "user", Password: "pass"}
			authCreds := loginAuthCreds(context.Background(), registryURL.Host, registryURL, credentials)
			username, password, err := authCreds(tt.acArg)
			if tt.wantErr {
				assert.ErrorContains(t, err, "expected acArg")
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, "user", username)
			assert.Equal(t, "pass", password)
		})
	}
}
