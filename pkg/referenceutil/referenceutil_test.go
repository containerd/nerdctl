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

package referenceutil

import (
	"testing"

	"github.com/opencontainers/go-digest"
	"gotest.tools/v3/assert"
)

func TestReferenceUtil(t *testing.T) {
	needles := map[string]struct {
		Error         string
		String        string
		Normalized    string
		Suggested     string
		FamiliarName  string
		FamiliarMatch map[string]bool
		Protocol      Protocol
		Digest        digest.Digest
		Path          string
		Domain        string
		Tag           string
		ExplicitTag   string
	}{
		"": {
			Error: "invalid reference format",
		},
		"∞": {
			Error: "invalid reference format",
		},
		"abcd:∞": {
			Error: "invalid reference format",
		},
		"abcd@sha256:∞": {
			Error: "invalid reference format",
		},
		"abcd@∞": {
			Error: "invalid reference format",
		},
		"abcd:foo@sha256:∞": {
			Error: "invalid reference format",
		},
		"abcd:foo@∞": {
			Error: "invalid reference format",
		},
		"sha256:whatever": {
			Error:        "",
			String:       "docker.io/library/sha256:whatever",
			Suggested:    "sha256-abcde",
			FamiliarName: "sha256",
			FamiliarMatch: map[string]bool{
				"*a*":                      true,
				"?ha25?":                   true,
				"[s-z]ha25[0-9]":           true,
				"[^a]ha25[^a-z]":           true,
				"*6:whatever":              true,
				"docker.io/library/sha256": false,
			},
			Protocol:    "",
			Digest:      "",
			Path:        "library/sha256",
			Domain:      "docker.io",
			Tag:         "whatever",
			ExplicitTag: "whatever",
		},
		"sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50": {
			Error:        "",
			String:       "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Normalized:   "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Suggested:    "untitled-abcde",
			FamiliarName: "",
			Protocol:     "",
			Digest:       "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Path:         "",
			Domain:       "",
			Tag:          "",
		},
		"4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50": {
			Error:        "",
			String:       "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Normalized:   "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Suggested:    "untitled-abcde",
			FamiliarName: "",
			Protocol:     "",
			Digest:       "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Path:         "",
			Domain:       "",
			Tag:          "",
		},
		"image_name": {
			Error:        "",
			String:       "docker.io/library/image_name:latest",
			Normalized:   "docker.io/library/image_name:latest",
			Suggested:    "image_name-abcde",
			FamiliarName: "image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "library/image_name",
			Domain:       "docker.io",
			Tag:          "latest",
			ExplicitTag:  "",
		},
		"library/image_name": {
			Error:        "",
			String:       "docker.io/library/image_name:latest",
			Normalized:   "docker.io/library/image_name:latest",
			Suggested:    "image_name-abcde",
			FamiliarName: "image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "library/image_name",
			Domain:       "docker.io",
			Tag:          "latest",
			ExplicitTag:  "",
		},
		"something/image_name": {
			Error:        "",
			String:       "docker.io/something/image_name:latest",
			Normalized:   "docker.io/something/image_name:latest",
			Suggested:    "image_name-abcde",
			FamiliarName: "something/image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "something/image_name",
			Domain:       "docker.io",
			Tag:          "latest",
			ExplicitTag:  "",
		},
		"docker.io/library/image_name": {
			Error:        "",
			String:       "docker.io/library/image_name:latest",
			Normalized:   "docker.io/library/image_name:latest",
			Suggested:    "image_name-abcde",
			FamiliarName: "image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "library/image_name",
			Domain:       "docker.io",
			Tag:          "latest",
			ExplicitTag:  "",
		},
		"image_name:latest": {
			Error:        "",
			String:       "docker.io/library/image_name:latest",
			Normalized:   "docker.io/library/image_name:latest",
			Suggested:    "image_name-abcde",
			FamiliarName: "image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "library/image_name",
			Domain:       "docker.io",
			Tag:          "latest",
			ExplicitTag:  "latest",
		},
		"image_name:foo": {
			Error:        "",
			String:       "docker.io/library/image_name:foo",
			Normalized:   "docker.io/library/image_name:foo",
			Suggested:    "image_name-abcde",
			FamiliarName: "image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "library/image_name",
			Domain:       "docker.io",
			Tag:          "foo",
			ExplicitTag:  "foo",
		},
		"image_name@sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50": {
			Error:        "",
			String:       "docker.io/library/image_name@sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Normalized:   "docker.io/library/image_name@sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Suggested:    "image_name-abcde",
			FamiliarName: "image_name",
			Protocol:     "",
			Digest:       "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Path:         "library/image_name",
			Domain:       "docker.io",
			Tag:          "",
			ExplicitTag:  "",
		},
		"image_name:latest@sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50": {
			Error:        "",
			String:       "docker.io/library/image_name:latest@sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Normalized:   "docker.io/library/image_name:latest@sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Suggested:    "image_name-abcde",
			FamiliarName: "image_name",
			Protocol:     "",
			Digest:       "sha256:4b826db5f1f14d1db0b560304f189d4b17798ddce2278b7822c9d32313fe3f50",
			Path:         "library/image_name",
			Domain:       "docker.io",
			Tag:          "latest",
			ExplicitTag:  "latest",
		},
		"ghcr.io:1234/image_name": {
			Error:        "",
			String:       "ghcr.io:1234/image_name:latest",
			Normalized:   "ghcr.io:1234/image_name:latest",
			Suggested:    "image_name-abcde",
			FamiliarName: "ghcr.io:1234/image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "image_name",
			Domain:       "ghcr.io:1234",
			Tag:          "latest",
			ExplicitTag:  "",
		},
		"ghcr.io/sub_name/image_name": {
			Error:        "",
			String:       "ghcr.io/sub_name/image_name:latest",
			Normalized:   "ghcr.io/sub_name/image_name:latest",
			Suggested:    "image_name-abcde",
			FamiliarName: "ghcr.io/sub_name/image_name",
			Protocol:     "",
			Digest:       "",
			Path:         "sub_name/image_name",
			Domain:       "ghcr.io",
			Tag:          "latest",
			ExplicitTag:  "",
		},
		"bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze": {
			Error:        "",
			String:       "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Normalized:   "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Suggested:    "ipfs-bafkr-abcde",
			FamiliarName: "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Protocol:     "ipfs",
			Digest:       "",
			Path:         "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Domain:       "",
			Tag:          "",
			ExplicitTag:  "",
		},
		"ipfs://bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze": {
			Error:        "",
			String:       "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Normalized:   "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Suggested:    "ipfs-bafkr-abcde",
			FamiliarName: "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Protocol:     "ipfs",
			Digest:       "",
			Path:         "bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze",
			Domain:       "",
			Tag:          "",
			ExplicitTag:  "",
		},
		"ipfs://ghcr.io/stargz-containers/alpine:3.13-org": {
			Error:        "",
			String:       "ghcr.io/stargz-containers/alpine:3.13-org",
			Normalized:   "ghcr.io/stargz-containers/alpine:3.13-org",
			Suggested:    "alpine-abcde",
			FamiliarName: "ghcr.io/stargz-containers/alpine",
			FamiliarMatch: map[string]bool{
				"ghcr.io/stargz-containers/alpine": true,
				"*/*/*":                            true,
				"*/*/*:3.13-org":                   true,
			},
			Protocol:    "ipfs",
			Digest:      "",
			Path:        "stargz-containers/alpine",
			Domain:      "ghcr.io",
			Tag:         "3.13-org",
			ExplicitTag: "3.13-org",
		},
		"ipfs://alpine": {
			Error:        "",
			String:       "docker.io/library/alpine:latest",
			Normalized:   "docker.io/library/alpine:latest",
			Suggested:    "alpine-abcde",
			FamiliarName: "alpine",
			Protocol:     "ipfs",
			Digest:       "",
			Path:         "library/alpine",
			Domain:       "docker.io",
			Tag:          "latest",
			ExplicitTag:  "",
		},
	}

	for k, v := range needles {
		parsed, err := Parse(k)
		if v.Error != "" || err != nil {
			assert.Error(t, err, v.Error)
			continue
		}
		assert.Equal(t, parsed.String(), v.String, k)
		assert.Equal(t, parsed.SuggestContainerName("abcdefghij"), v.Suggested, k)
		assert.Equal(t, parsed.FamiliarName(), v.FamiliarName, k)
		for needle, result := range v.FamiliarMatch {
			res, err := parsed.FamiliarMatch(needle)
			assert.NilError(t, err)
			assert.Equal(t, res, result, k)
		}

		assert.Equal(t, parsed.Protocol, v.Protocol, k)
		assert.Equal(t, parsed.Digest, v.Digest, k)
		assert.Equal(t, parsed.Path, v.Path, k)
		assert.Equal(t, parsed.Domain, v.Domain, k)
		assert.Equal(t, parsed.Tag, v.Tag, k)
		assert.Equal(t, parsed.ExplicitTag, v.ExplicitTag, k)
	}
}
