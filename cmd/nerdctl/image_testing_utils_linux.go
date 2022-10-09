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
	"fmt"
	"os"

	"github.com/containerd/nerdctl/pkg/testutil"
)

const (
	testImageCmdEchoContents = "nerdctl-linux-test-image"
)

// createTestImageByExtendingCommonImage builds a new image with the given identifier
// by adding a trivial later on top of the hardcoded `testutil.CommonImage`.
func createTestImageByExtendingCommonImage(testingBase *testutil.Base, imageIdentifier string) error {
	testutil.RequiresBuild(testingBase.T)

	dockerfile := fmt.Sprintf(`FROM %s
	CMD ["echo", "%s"]`, testutil.CommonImage, testImageCmdEchoContents)

	buildCtx, err := createBuildContext(dockerfile)
	if err != nil {
		return err
	}
	defer testingBase.Cmd("builder", "prune").Run()
	defer os.RemoveAll(buildCtx)

	testingBase.Cmd("build", "-t", imageIdentifier, buildCtx).AssertOK()
	return nil
}
