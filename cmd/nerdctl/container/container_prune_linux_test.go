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

package container

import (
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestPruneContainer(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	tearDown := func() {
		defer base.Cmd("rm", "-f", tID+"-1").Run()
		defer base.Cmd("rm", "-f", tID+"-2").Run()
	}

	tearUp := func() {
		base.Cmd("run", "-d", "--name", tID+"-1", "-v", "/anonymous", testutil.CommonImage, "sleep", "infinity").AssertOK()
		base.Cmd("exec", tID+"-1", "touch", "/anonymous/foo").AssertOK()
		base.Cmd("create", "--name", tID+"-2", testutil.CommonImage, "sleep", "infinity").AssertOK()
	}

	tearDown()
	t.Cleanup(tearDown)
	tearUp()

	base.Cmd("container", "prune", "-f").AssertOK()
	// tID-1 is still running, tID-2 is not
	base.Cmd("inspect", tID+"-1").AssertOK()
	base.Cmd("inspect", tID+"-2").AssertFail()

	// https://github.com/containerd/nerdctl/issues/3134
	base.Cmd("exec", tID+"-1", "ls", "-lA", "/anonymous/foo").AssertOK()

	base.Cmd("kill", tID+"-1").AssertOK()
	base.Cmd("container", "prune", "-f").AssertOK()
	base.Cmd("inspect", tID+"-1").AssertFail()
}
