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

func TestRename(t *testing.T) {
	t.Parallel()
	testContainerName := testutil.Identifier(t)
	base := testutil.NewBase(t)

	defer base.Cmd("rm", "-f", testContainerName).Run()
	base.Cmd("run", "-d", "--name", testContainerName, testutil.CommonImage, "sleep", "infinity").AssertOK()

	defer base.Cmd("rm", "-f", testContainerName+"_new").Run()
	base.Cmd("rename", testContainerName, testContainerName+"_new").AssertOK()
	base.Cmd("ps", "-a").AssertOutContains(testContainerName + "_new")
	base.Cmd("rename", testContainerName, testContainerName+"_new").AssertFail()
	base.Cmd("rename", testContainerName+"_new", testContainerName+"_new").AssertFail()
}

func TestRenameUpdateHosts(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	testContainerName := testutil.Identifier(t)
	base := testutil.NewBase(t)

	defer base.Cmd("rm", "-f", testContainerName).Run()
	base.Cmd("run", "-d", "--name", testContainerName, testutil.CommonImage, "sleep", "infinity").AssertOK()
	base.EnsureContainerStarted(testContainerName)

	defer base.Cmd("rm", "-f", testContainerName+"_1").Run()
	base.Cmd("run", "-d", "--name", testContainerName+"_1", testutil.CommonImage, "sleep", "infinity").AssertOK()
	base.EnsureContainerStarted(testContainerName + "_1")

	defer base.Cmd("rm", "-f", testContainerName+"_new").Run()
	base.Cmd("exec", testContainerName, "cat", "/etc/hosts").AssertOutContains(testContainerName + "_1")
	base.Cmd("rename", testContainerName, testContainerName+"_new").AssertOK()
	base.Cmd("exec", testContainerName+"_new", "cat", "/etc/hosts").AssertOutContains(testContainerName + "_new")
	base.Cmd("exec", testContainerName+"_1", "cat", "/etc/hosts").AssertOutContains(testContainerName + "_new")
}
