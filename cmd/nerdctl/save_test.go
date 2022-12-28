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
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestSaveById(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	inspect := base.InspectImage(testutil.CommonImage)
	var id string
	if testutil.GetTarget() == testutil.Docker {
		id = inspect.ID
	} else {
		id = strings.Split(inspect.RepoDigests[0], ":")[1]
	}
	archiveTarPath := filepath.Join(t.TempDir(), "id.tar")
	base.Cmd("save", "-o", archiveTarPath, id).AssertOK()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()
	base.Cmd("load", "-i", archiveTarPath).AssertOK()
	base.Cmd("run", "--rm", id, "sh", "-euxc", "echo foo").AssertOK()
}

func TestSaveByIdWithDifferentNames(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	inspect := base.InspectImage(testutil.CommonImage)
	var id string
	if testutil.GetTarget() == testutil.Docker {
		id = inspect.ID
	} else {
		id = strings.Split(inspect.RepoDigests[0], ":")[1]
	}

	base.Cmd("tag", testutil.CommonImage, "foobar").AssertOK()

	archiveTarPath := filepath.Join(t.TempDir(), "id.tar")
	base.Cmd("save", "-o", archiveTarPath, id).AssertOK()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()
	base.Cmd("load", "-i", archiveTarPath).AssertOK()
	base.Cmd("run", "--rm", id, "sh", "-euxc", "echo foo").AssertOK()
}
