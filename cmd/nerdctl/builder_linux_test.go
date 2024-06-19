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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestBuilderDebug(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-builder-debug-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("builder", "debug", buildCtx).CmdOption(testutil.WithStdin(bytes.NewReader([]byte("c\n")))).AssertOK()
}

func TestBuildWithPull(t *testing.T) {
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)

	oldImage := testutil.BusyboxImage
	oldImageSha := "141c253bc4c3fd0a201d32dc1f493bcf3fff003b6df416dea4f41046e0f37d47"
	newImage := testutil.AlpineImage

	buildkitConfig := fmt.Sprintf(`[worker.oci]
enabled = false

[worker.containerd]
enabled = true
namespace = "%s"`, testutil.Namespace)

	cleanup := useBuildkitConfig(t, buildkitConfig)
	defer cleanup()

	testCases := []struct {
		name string
		pull string
	}{
		{
			name: "build with local image",
			pull: "false",
		},
		{
			name: "build with newest image",
			pull: "true",
		},
		{
			name: "build with buildkit default",
			// buildkit default pulls from remote
			pull: "default",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			base := testutil.NewBase(t)
			defer base.Cmd("builder", "prune").AssertOK()
			base.Cmd("image", "prune", "--force", "--all").AssertOK()

			base.Cmd("pull", oldImage).Run()
			base.Cmd("tag", oldImage, newImage).Run()

			dockerfile := fmt.Sprintf(`FROM %s`, newImage)
			tmpDir := t.TempDir()
			err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644)
			assert.NilError(t, err)

			buildCtx, err := createBuildContext(dockerfile)
			if err != nil {
				t.Fatal(err)
			}

			buildCmd := []string{"build", buildCtx}
			switch tc.pull {
			case "false":
				buildCmd = append(buildCmd, "--pull=false")
				base.Cmd(buildCmd...).AssertErrContains(oldImageSha)
			case "true":
				buildCmd = append(buildCmd, "--pull=true")
				base.Cmd(buildCmd...).AssertErrNotContains(oldImageSha)
			case "default":
				base.Cmd(buildCmd...).AssertErrNotContains(oldImageSha)
			}
		})
	}
}

func useBuildkitConfig(t *testing.T, config string) (cleanup func()) {
	buildkitConfigPath := "/etc/buildkit/buildkitd.toml"

	currConfig, err := exec.Command("cat", buildkitConfigPath).Output()
	assert.NilError(t, err)

	os.WriteFile(buildkitConfigPath, []byte(config), 0644)
	_, err = exec.Command("systemctl", "restart", "buildkit").Output()
	assert.NilError(t, err)

	return func() {
		assert.NilError(t, os.WriteFile(buildkitConfigPath, currConfig, 0644))
		_, err = exec.Command("systemctl", "restart", "buildkit").Output()
		assert.NilError(t, err)
	}
}
