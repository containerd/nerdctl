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

// Ensures Dockerfile VOLUME mount is properly set up and had the volume's files copied within.
func TestRunCopyingUpInitialContentsOnDockerfileVolume(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	containerName := tID
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	cmd := base.Cmd(
		"run", "-d",
		"--name", containerName,
		testutil.WindowsVolumeMountImage,
		"sleep 100")

	// TODO: there is currently a known issue with the FS driver in the OCI
	// spec on Windows, so we expect a failure for now.
	// https://github.com/containerd/nerdctl/pull/924#discussion_r871002561
	cmd.AssertFail()

	// NOTE: the testing image should declare a VOLUME mount on "C:\\test_dir".
	// https://github.com/containerd/containerd/blob/main/integration/images/volume-copy-up/Dockerfile_windows
	// base.Cmd("exec", containerName, "cat", "C:\\test_dir\\test_file").AssertOutExactly("test_content\n")
}
