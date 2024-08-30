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

package image

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestImageInspectContainsSomeStuff(t *testing.T) {
	base := testutil.NewBase(t)

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	inspect := base.InspectImage(testutil.CommonImage)

	assert.Assert(base.T, len(inspect.RootFS.Layers) > 0)
	assert.Assert(base.T, inspect.RootFS.Type != "")
	assert.Assert(base.T, inspect.Architecture != "")
	assert.Assert(base.T, inspect.Size > 0)
}

func TestImageInspectWithFormat(t *testing.T) {
	base := testutil.NewBase(t)

	base.Cmd("pull", testutil.CommonImage).AssertOK()

	// test RawFormat support
	base.Cmd("image", "inspect", testutil.CommonImage, "--format", "{{.Id}}").AssertOK()

	// test typedFormat support
	base.Cmd("image", "inspect", testutil.CommonImage, "--format", "{{.ID}}").AssertOK()
}

func inspectImageHelper(base *testutil.Base, identifier ...string) []dockercompat.Image {
	args := append([]string{"image", "inspect"}, identifier...)
	cmdResult := base.Cmd(args...).Run()
	assert.Equal(base.T, cmdResult.ExitCode, 0)
	var dc []dockercompat.Image
	if err := json.Unmarshal([]byte(cmdResult.Stdout()), &dc); err != nil {
		base.T.Fatal(err)
	}
	return dc
}

func TestImageInspectDifferentValidReferencesForTheSameImage(t *testing.T) {
	testutil.DockerIncompatible(t)

	if runtime.GOOS == "windows" {
		t.Skip("Windows is not supported for this test right now")
	}

	base := testutil.NewBase(t)

	// Overall, we need a clean slate before doing these lookups.
	// More specifically, because we trigger https://github.com/containerd/nerdctl/issues/3016
	// we cannot do selective rmi, so, just nuke everything
	ids := base.Cmd("image", "list", "-q").Out()
	allIDs := strings.Split(ids, "\n")
	for _, id := range allIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			base.Cmd("rmi", "-f", id).Run()
		}
	}

	base.Cmd("pull", "alpine", "--platform", "linux/amd64").AssertOK()
	base.Cmd("pull", "busybox", "--platform", "linux/amd64").AssertOK()
	base.Cmd("pull", "busybox:stable", "--platform", "linux/amd64").AssertOK()
	base.Cmd("pull", "registry-1.docker.io/library/busybox", "--platform", "linux/amd64").AssertOK()
	base.Cmd("pull", "registry-1.docker.io/library/busybox:stable", "--platform", "linux/amd64").AssertOK()

	tags := []string{
		"",
		":latest",
		":stable",
	}
	names := []string{
		"busybox",
		"library/busybox",
		"docker.io/library/busybox",
		"registry-1.docker.io/library/busybox",
	}

	// Build reference values for comparison
	reference := inspectImageHelper(base, "busybox")
	assert.Equal(base.T, 1, len(reference))
	// Extract image sha
	sha := strings.TrimPrefix(reference[0].RepoDigests[0], "busybox@sha256:")

	differentReference := inspectImageHelper(base, "alpine")
	assert.Equal(base.T, 1, len(differentReference))

	// Testing all name and tags variants
	for _, name := range names {
		for _, tag := range tags {
			t.Logf("Testing %s", name+tag)
			result := inspectImageHelper(base, name+tag)
			assert.Equal(base.T, 1, len(result))
			assert.Equal(base.T, reference[0].ID, result[0].ID)
		}
	}

	// Testing all name and tags variants, with a digest
	for _, name := range names {
		for _, tag := range tags {
			t.Logf("Testing %s", name+tag+"@"+sha)
			result := inspectImageHelper(base, name+tag+"@sha256:"+sha)
			assert.Equal(base.T, 1, len(result))
			assert.Equal(base.T, reference[0].ID, result[0].ID)
		}
	}

	// Testing repo digest and short digest with or without prefix
	for _, id := range []string{"sha256:" + sha, sha, sha[0:8], "sha256:" + sha[0:8]} {
		t.Logf("Testing %s", id)
		result := inspectImageHelper(base, id)
		assert.Equal(base.T, 1, len(result))
		assert.Equal(base.T, reference[0].ID, result[0].ID)
	}

	// Demonstrate image name precedence over digest lookup
	// Using the shortened sha should no longer get busybox, but rather the newly tagged Alpine
	t.Logf("Testing (alpine tagged) %s", sha[0:8])
	// Tag a different image with the short id
	base.Cmd("tag", "alpine", sha[0:8]).AssertOK()
	result := inspectImageHelper(base, sha[0:8])
	assert.Equal(base.T, 1, len(result))
	assert.Equal(base.T, differentReference[0].ID, result[0].ID)

	// Prove that wrong references with an existing digest do not get retrieved when asking by digest
	for _, id := range []string{"doesnotexist", "doesnotexist:either", "busybox:bogustag"} {
		t.Logf("Testing %s", id+"@"+sha)
		args := append([]string{"image", "inspect"}, id+"@"+sha)
		cmdResult := base.Cmd(args...).Run()
		assert.Equal(base.T, cmdResult.ExitCode, 0)
		assert.Equal(base.T, cmdResult.Stdout(), "")
	}

	// Prove that invalid reference return no result without crashing
	for _, id := range []string{"∞∞∞∞∞∞∞∞∞∞", "busybox:∞∞∞∞∞∞∞∞∞∞"} {
		t.Logf("Testing %s", id)
		args := append([]string{"image", "inspect"}, id)
		cmdResult := base.Cmd(args...).Run()
		assert.Equal(base.T, cmdResult.ExitCode, 0)
		assert.Equal(base.T, cmdResult.Stdout(), "")
	}

	// Retrieving multiple entries at once
	t.Logf("Testing %s", "busybox busybox busybox:stable")
	result = inspectImageHelper(base, "busybox", "busybox", "busybox:stable")
	assert.Equal(base.T, 3, len(result))
	assert.Equal(base.T, reference[0].ID, result[0].ID)
	assert.Equal(base.T, reference[0].ID, result[1].ID)
	assert.Equal(base.T, reference[0].ID, result[2].ID)

}
