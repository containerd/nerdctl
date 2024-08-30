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
	"fmt"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestImagesWithNames(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	base.Cmd("images", "--names", testutil.CommonImage).AssertOutContains(testutil.CommonImage)
	base.Cmd("images", "--names", testutil.CommonImage).AssertOutWithFunc(func(out string) error {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}
		tab := tabutil.NewReader("NAME\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		name, _ := tab.ReadRow(lines[1], "NAME")
		assert.Equal(t, name, testutil.CommonImage)
		return nil
	})
}

func TestImages(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	header := "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE"
	if base.Target == testutil.Docker {
		header = "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE"
	}

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	base.Cmd("images", testutil.CommonImage).AssertOutWithFunc(func(out string) error {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}
		tab := tabutil.NewReader(header)
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		repo, _ := tab.ReadRow(lines[1], "REPOSITORY")
		tag, _ := tab.ReadRow(lines[1], "TAG")
		assert.Equal(t, repo+":"+tag, testutil.CommonImage)
		return nil
	})
}

func TestImagesFilter(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	t.Parallel()
	base := testutil.NewBase(t)
	tempName := testutil.Identifier(base.T)
	base.Cmd("pull", testutil.CommonImage).AssertOK()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"] \n
LABEL foo=bar
LABEL version=0.1`, testutil.CommonImage)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)
	base.Cmd("build", "-t", tempName, "-f", buildCtx+"/Dockerfile", buildCtx).AssertOK()
	defer base.Cmd("rmi", tempName).AssertOK()

	busyboxGlibc, busyboxUclibc := "busybox:glibc", "busybox:uclibc"
	base.Cmd("pull", busyboxGlibc).AssertOK()
	defer base.Cmd("rmi", busyboxGlibc).AssertOK()

	base.Cmd("pull", busyboxUclibc).AssertOK()
	defer base.Cmd("rmi", busyboxUclibc).AssertOK()

	// before/since filters are not compatible with DOCKER_BUILDKIT=1? (but still compatible with DOCKER_BUILDKIT=0)
	if base.Target == testutil.Nerdctl {
		base.Cmd("images", "--filter", fmt.Sprintf("before=%s:%s", tempName, "latest")).AssertOutContains(testutil.ImageRepo(testutil.CommonImage))
		base.Cmd("images", "--filter", fmt.Sprintf("before=%s:%s", tempName, "latest")).AssertOutNotContains(tempName)
		base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage)).AssertOutContains(tempName)
		base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage)).AssertOutNotContains(testutil.ImageRepo(testutil.CommonImage))
		base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage), testutil.CommonImage).AssertOutNotContains(testutil.ImageRepo(testutil.CommonImage))
		base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage), testutil.CommonImage).AssertOutNotContains(tempName)
	}
	base.Cmd("images", "--filter", "label=foo=bar").AssertOutContains(tempName)
	base.Cmd("images", "--filter", "label=foo=bar1").AssertOutNotContains(tempName)
	base.Cmd("images", "--filter", "label=foo=bar", "--filter", "label=version=0.1").AssertOutContains(tempName)
	base.Cmd("images", "--filter", "label=foo=bar", "--filter", "label=version=0.2").AssertOutNotContains(tempName)
	base.Cmd("images", "--filter", "label=version").AssertOutContains(tempName)
	base.Cmd("images", "--filter", fmt.Sprintf("reference=%s*", tempName)).AssertOutContains(tempName)
	base.Cmd("images", "--filter", "reference=busy*:*libc*").AssertOutContains("glibc")
	base.Cmd("images", "--filter", "reference=busy*:*libc*").AssertOutContains("uclibc")
}

func TestImagesFilterDangling(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	base := testutil.NewBase(t)
	base.Cmd("images", "prune", "--all").AssertOK()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-notag-string"]
	`, testutil.CommonImage)
	buildCtx := helpers.CreateBuildContext(t, dockerfile)

	base.Cmd("build", "-f", buildCtx+"/Dockerfile", buildCtx).AssertOK()

	// dangling image test
	base.Cmd("images", "--filter", "dangling=true").AssertOutContains("<none>")
	base.Cmd("images", "--filter", "dangling=false").AssertOutNotContains("<none>")
}

func TestImageListCheckCreatedTime(t *testing.T) {
	base := testutil.NewBase(t)

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	base.Cmd("pull", testutil.NginxAlpineImage).AssertOK()

	var createdTimes []string

	base.Cmd("images", "--format", "'{{json .CreatedAt}}'").AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 4 lines, got %d", len(lines))
		}
		createdTimes = append(createdTimes, lines...)
		return nil
	})

	slices.Reverse(createdTimes)
	if !slices.IsSorted(createdTimes) {
		t.Errorf("expected images in decending order")
	}
}
