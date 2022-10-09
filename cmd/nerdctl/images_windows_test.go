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

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestImagesFilter(t *testing.T) {
	// NOTE: lacking BuildKit support on Windows, there is no straightforward way to
	// run this test with Docker on Windows because:
	// - image format convertsion is nerdctl-specific
	// - creating a test image by re-tagging on Docker yields the same underlying image
	// 	 object, resulting in identical CreatedAt tags (which this test relies on)
	testutil.DockerIncompatible(t)
	baseTestImagesFilter(t, createTestImageByConvertingCommonImage)
}
