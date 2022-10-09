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
	"github.com/containerd/nerdctl/pkg/testutil"
)

const (
	testImageCmdEchoContents = "nerdctl-windows-test-image"
)

// createTestImageByConvertingCommonImage simply converts the hardcoded `testutil.CommonImage` into
// a different format with the provided identifier.
// This is a workaround to allow for certain non-image-building-related tests to run
// on Windows until BuildKit support is available, but extends testing times since the conversion
// process is generally slower than building a new image with a trivial layer on top.
func createTestImageByConvertingCommonImage(testingBase *testutil.Base, imageIdentifier string) error {
	testutil.DockerIncompatible(testingBase.T)
	// NOTE: this will successfully create a new image encry even if the source/target formats coincide:
	testingBase.Cmd("image", "convert", "--estargz", "--oci", testutil.CommonImage, imageIdentifier).AssertOK()
	return nil
}

// createTestImageByTaggingCommonImage simply re-tags the hardcoded `testutil.CommonImage`
// This is a workaround to allow for certain non-image-building-related tests to run
// on Windows until BuildKit support is available.
// While this image will NOT be usable on a lot of tests (Docker doesn't actually create a
// separate image entity when you re-tag one), it will still serve as a testing option
// in trivial commands which work on image metadata (e.g. `images --filer` etc...)
func createTestImageByTaggingCommonImage(testingBase *testutil.Base, imageIdentifier string) error {
	testingBase.Cmd("tag", testutil.CommonImage, imageIdentifier).AssertOK()
	return nil
}
