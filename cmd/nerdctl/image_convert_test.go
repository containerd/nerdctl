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
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestImageConvert(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no windows support yet")
	}
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	t.Parallel()

	base.Cmd("pull", testutil.CommonImage).AssertOK()

	testCases := []struct {
		identifier string
		args       []string
	}{
		{
			"esgz",
			[]string{"--estargz"},
		},
		{
			"zstd",
			[]string{"--zstd", "--zstd-compression-level", "3"},
		},
		{
			"zstdchunked",
			[]string{"--zstdchunked", "--zstdchunked-compression-level", "3"},
		},
	}

	for _, tc := range testCases {
		convertedImage := testutil.Identifier(t) + ":" + tc.identifier
		args := append([]string{"image", "convert", "--oci"}, tc.args...)
		args = append(args, testutil.CommonImage, convertedImage)

		t.Run(tc.identifier, func(t *testing.T) {
			t.Parallel()

			base.Cmd("rmi", convertedImage).Run()
			t.Cleanup(func() {
				base.Cmd("rmi", convertedImage).Run()
			})

			base.Cmd(args...).AssertOK()
		})
	}
}
