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

package integration

import (
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestImageConvertEStargz(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no windows support yet")
	}
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	convertedImage := testutil.Identifier(t) + ":esgz"
	base.Cmd("rmi", convertedImage).Run()
	defer base.Cmd("rmi", convertedImage).Run()
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	base.Cmd("image", "convert", "--estargz", "--oci",
		testutil.CommonImage, convertedImage).AssertOK()
}

func TestImageConvertZstdChunked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no windows support yet")
	}
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	convertedImage := testutil.Identifier(t) + ":zstdchunked"
	base.Cmd("rmi", convertedImage).Run()
	defer base.Cmd("rmi", convertedImage).Run()
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	base.Cmd("image", "convert", "--zstdchunked", "--oci", "--zstdchunked-compression-level", "3",
		testutil.CommonImage, convertedImage).AssertOK()
}
