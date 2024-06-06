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

package dockerutil

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestURLParsingAndID(t *testing.T) {
	tests := []struct {
		address     string
		error       error
		identifier  string
		allIDs      []string
		isLocalhost bool
	}{
		{
			address: "∞://",
			error:   ErrUnparsableURL,
		},
		{
			address: "whatever://",
			error:   ErrUnsupportedScheme,
		},
		{
			address:    "https://index.docker.io/v1/",
			identifier: "https://index.docker.io/v1/",
			allIDs:     []string{"https://index.docker.io/v1/"},
		},
		{
			address:    "index.docker.io",
			identifier: "index.docker.io:443",
			allIDs: []string{
				"index.docker.io:443",
				"https://index.docker.io/v1/",
				"https://index.docker.io:443", "http://index.docker.io:443",
				"index.docker.io", "https://index.docker.io", "http://index.docker.io",
			},
		},
		{
			address:    "index.docker.io/whatever",
			identifier: "index.docker.io:443",
			allIDs: []string{
				"index.docker.io:443",
				"https://index.docker.io/v1/",
				"https://index.docker.io:443", "http://index.docker.io:443",
				"index.docker.io", "https://index.docker.io", "http://index.docker.io",
			},
		},
		{
			address:    "http://index.docker.io",
			identifier: "index.docker.io:443",
			allIDs: []string{
				"index.docker.io:443",
				"https://index.docker.io/v1/",
				"https://index.docker.io:443", "http://index.docker.io:443",
				"index.docker.io", "https://index.docker.io", "http://index.docker.io",
			},
		},
		{
			address:    "index.docker.io:80",
			identifier: "index.docker.io:80",
			allIDs: []string{
				"index.docker.io:80",
				"https://index.docker.io:80", "http://index.docker.io:80",
			},
		},
		{
			address:    "index.docker.io:8080",
			identifier: "index.docker.io:8080",
			allIDs: []string{
				"index.docker.io:8080",
				"https://index.docker.io:8080", "http://index.docker.io:8080",
			},
		},
		{
			address:    "foo.docker.io",
			identifier: "foo.docker.io:443",
			allIDs: []string{
				"foo.docker.io:443", "https://foo.docker.io:443", "http://foo.docker.io:443",
				"foo.docker.io", "https://foo.docker.io", "http://foo.docker.io",
			},
		},
		{
			address:    "docker.io",
			identifier: "https://index.docker.io/v1/",
			allIDs:     []string{"https://index.docker.io/v1/"},
		},
		{
			address:    "docker.io/whatever",
			identifier: "docker.io:443",
			allIDs: []string{
				"docker.io:443", "https://docker.io:443", "http://docker.io:443",
				"docker.io", "https://docker.io", "http://docker.io",
			},
		},
		{
			address:    "http://docker.io",
			identifier: "docker.io:443",
			allIDs: []string{
				"docker.io:443", "https://docker.io:443", "http://docker.io:443",
				"docker.io", "https://docker.io", "http://docker.io",
			},
		},
		{
			address:    "docker.io:80",
			identifier: "docker.io:80",
			allIDs: []string{
				"docker.io:80",
				"https://docker.io:80", "http://docker.io:80",
			},
		},
		{
			address:    "docker.io:8080",
			identifier: "docker.io:8080",
			allIDs: []string{
				"docker.io:8080",
				"https://docker.io:8080", "http://docker.io:8080",
			},
		},
		{
			address:    "anything/whatever?u=v&w=y;foo=bar#frag=o",
			identifier: "anything:443",
			allIDs: []string{
				"anything:443", "https://anything:443", "http://anything:443",
				"anything", "https://anything", "http://anything",
			},
		},
		{
			address:    "https://registry-host.com/subpath/something?bar=bar&ns=registry-namespace.com&foo=foo",
			identifier: "nerdctl-experimental://registry-namespace.com:443/host/registry-host.com:443/subpath/something",
			allIDs: []string{
				"nerdctl-experimental://registry-namespace.com:443/host/registry-host.com:443/subpath/something",
			},
		},
		{
			address:    "localhost:1234",
			identifier: "localhost:1234",
			allIDs: []string{
				"localhost:1234", "https://localhost:1234", "http://localhost:1234",
			},
		},
		{
			address:    "127.0.0.1:1234",
			identifier: "127.0.0.1:1234",
			allIDs: []string{
				"127.0.0.1:1234", "https://127.0.0.1:1234", "http://127.0.0.1:1234",
			},
		},
		{
			address:    "[::1]:1234",
			identifier: "[::1]:1234",
			allIDs: []string{
				"[::1]:1234", "https://[::1]:1234", "http://[::1]:1234",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.address, func(t *testing.T) {
			reg, err := Parse(tc.address)
			assert.ErrorIs(t, err, tc.error)
			if err == nil {
				assert.Equal(t, reg.CanonicalIdentifier(), tc.identifier)
				allIDs := reg.AllIdentifiers()
				assert.Equal(t, len(allIDs), len(tc.allIDs))
				for k, v := range tc.allIDs {
					assert.Equal(t, allIDs[k], v)
				}
			}
		})
	}
}
