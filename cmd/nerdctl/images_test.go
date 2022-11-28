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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/tabutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
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
	t.Parallel()
	base := testutil.NewBase(t)
	tempName := testutil.Identifier(base.T)
	base.Cmd("pull", testutil.CommonImage).AssertOK()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"] \n
LABEL foo=bar
LABEL version=0.1`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)
	base.Cmd("build", "-t", tempName, "-f", buildCtx+"/Dockerfile", buildCtx).AssertOK()
	base.Cmd("images", "--filter", fmt.Sprintf("before=%s:%s", tempName, "latest")).AssertOutContains(strings.Split(testutil.CommonImage, ":")[0])
	base.Cmd("images", "--filter", fmt.Sprintf("before=%s:%s", tempName, "latest")).AssertOutNotContains(tempName)
	base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage)).AssertOutContains(tempName)
	base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage)).AssertOutNotContains(strings.Split(testutil.CommonImage, ":")[0])
	base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage), testutil.CommonImage).AssertOutNotContains(strings.Split(testutil.CommonImage, ":")[0])
	base.Cmd("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage), testutil.CommonImage).AssertOutNotContains(tempName)
	base.Cmd("images", "--filter", "label=foo=bar").AssertOutContains(tempName)
	base.Cmd("images", "--filter", "label=foo=bar1").AssertOutNotContains(tempName)
	base.Cmd("images", "--filter", "label=foo=bar", "--filter", "label=version=0.1").AssertOutContains(tempName)
	base.Cmd("images", "--filter", "label=foo=bar", "--filter", "label=version=0.2").AssertOutNotContains(tempName)
	base.Cmd("images", "--filter", "label=version").AssertOutContains(tempName)
}
