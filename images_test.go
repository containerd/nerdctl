/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
)

func TestParseRepoTag(t *testing.T) {
	type testCase struct {
		imgName string
		repo    string
		tag     string
	}
	testCases := []testCase{
		{
			imgName: "127.0.0.1:5000/foo/bar:baz",
			repo:    "127.0.0.1:5000/foo/bar",
			tag:     "baz",
		},
		{
			imgName: "docker.io/library/alpine:latest",
			repo:    "alpine",
			tag:     "latest",
		},
		{
			imgName: "docker.io/foo/bar:baz",
			repo:    "foo/bar",
			tag:     "baz",
		},
		{
			imgName: "overlayfs@sha256:da203733d47434b9e8b4d3f70e1c0c3ea59438353252fe600cb9eb1a1e808c4f",
			repo:    "",
			tag:     "",
		},
	}
	for _, tc := range testCases {
		repo, tag := parseRepoTag(tc.imgName)
		assert.Equal(t, tc.repo, repo)
		assert.Equal(t, tc.tag, tag)
	}
}
