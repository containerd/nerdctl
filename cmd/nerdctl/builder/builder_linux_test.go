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

package builder

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestBuilderPrune(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)

	base := testutil.NewBase(t)

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-test-builder-prune"]`, testutil.CommonImage)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)

	testCases := []struct {
		name        string
		commandArgs []string
	}{
		{
			name:        "TestBuilderPruneForce",
			commandArgs: []string{"builder", "prune", "--force"},
		},
		{
			name:        "TestBuilderPruneForceAll",
			commandArgs: []string{"builder", "prune", "--force", "--all"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			base.Cmd("build", buildCtx).AssertOK()
			base.Cmd(tc.commandArgs...).AssertOK()
		})
	}
}

func TestBuilderDebug(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-builder-debug-test-string"]
	`, testutil.CommonImage)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)

	base.Cmd("builder", "debug", buildCtx).CmdOption(testutil.WithStdin(bytes.NewReader([]byte("c\n")))).AssertOK()
}

func TestBuildWithPull(t *testing.T) {
	testutil.DockerIncompatible(t)
	if rootlessutil.IsRootless() {
		t.Skipf("skipped because the test needs a custom buildkitd config")
	}
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)

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
			testutil.RegisterBuildCacheCleanup(t)
			base := testutil.NewBase(t)
			base.Cmd("image", "prune", "--force", "--all").AssertOK()

			base.Cmd("pull", oldImage).Run()
			base.Cmd("tag", oldImage, newImage).Run()

			dockerfile := fmt.Sprintf(`FROM %s`, newImage)
			tmpDir := t.TempDir()
			err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644)
			assert.NilError(t, err)

			buildCtx := helpers.CreateBuildContext(t, dockerfile)

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
