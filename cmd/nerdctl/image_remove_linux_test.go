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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestRemoveImage(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	base.Cmd("image", "prune", "--force", "--all").AssertOK()

	// ignore error
	base.Cmd("rmi", "-f", tID).AssertOK()

	base.Cmd("run", "--name", tID, testutil.CommonImage).AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	defer base.Cmd("rmi", "-f", testutil.CommonImage).Run()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()

	base.Cmd("images").AssertNoOut(testutil.ImageRepo(testutil.CommonImage))
}

func TestRemoveRunningImage(t *testing.T) {
	// If an image is associated with a running/paused containers, `docker rmi -f imageName`
	// untags `imageName` (left a `<none>` image) without deletion; `docker rmi -rf imageID` fails.
	// In both cases, `nerdctl rmi -f` will fail.
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("run", "--name", tID, "-d", testutil.CommonImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertFail()
	base.Cmd("images").AssertOutContains(testutil.ImageRepo(testutil.CommonImage))

	base.Cmd("kill", tID).AssertOK()
	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()
	base.Cmd("images").AssertNoOut(testutil.ImageRepo(testutil.CommonImage))
}

func TestRemovePausedImage(t *testing.T) {
	// If an image is associated with a running/paused containers, `docker rmi -f imageName`
	// untags `imageName` (left a `<none>` image) without deletion; `docker rmi -rf imageID` fails.
	// In both cases, `nerdctl rmi -f` will fail.
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	switch base.Info().CgroupDriver {
	case "none", "":
		t.Skip("requires cgroup (for pausing)")
	}
	tID := testutil.Identifier(t)

	base.Cmd("run", "--name", tID, "-d", testutil.CommonImage, "sleep", "infinity").AssertOK()
	base.Cmd("pause", tID).AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertFail()
	base.Cmd("images").AssertOutContains(testutil.ImageRepo(testutil.CommonImage))

	base.Cmd("kill", tID).AssertOK()
	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()
	base.Cmd("images").AssertNoOut(testutil.ImageRepo(testutil.CommonImage))
}

func TestRemoveImageWithCreatedContainer(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	base.Cmd("pull", testutil.NginxAlpineImage).AssertOK()

	base.Cmd("create", "--name", tID, testutil.AlpineImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.AlpineImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.AlpineImage).AssertOK()
	base.Cmd("images").AssertNoOut(testutil.ImageRepo(testutil.AlpineImage))

	// a created container with removed image doesn't impact other `rmi` command
	base.Cmd("rmi", "-f", testutil.NginxAlpineImage).AssertOK()
	base.Cmd("images").AssertNoOut(testutil.ImageRepo(testutil.NginxAlpineImage))
}
