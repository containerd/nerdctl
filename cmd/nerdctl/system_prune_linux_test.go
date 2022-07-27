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
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestSystemPrune(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("container", "prune", "-f").AssertOK()
	base.Cmd("network", "prune", "-f").AssertOK()
	base.Cmd("volume", "prune", "-f").AssertOK()
	base.Cmd("image", "prune", "-f", "--all").AssertOK()

	nID := testutil.Identifier(t)
	base.Cmd("network", "create", nID).AssertOK()
	defer base.Cmd("network", "rm", nID).Run()

	vID := testutil.Identifier(t)
	base.Cmd("volume", "create", vID).AssertOK()
	defer base.Cmd("volume", "rm", vID).Run()

	tID := testutil.Identifier(t)
	base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", vID), "--net", nID,
		"--name", tID, testutil.CommonImage).AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()

	base.Cmd("ps", "-a").AssertOutContains(tID)
	base.Cmd("images").AssertOutContains("alpine")

	base.Cmd("system", "prune", "-f", "--volumes", "--all").AssertOK()
	base.Cmd("volume", "ls").AssertNoOut(vID)
	base.Cmd("ps", "-a").AssertNoOut(tID)
	base.Cmd("network", "ls").AssertNoOut(nID)
	base.Cmd("images").AssertNoOut("alpine")
}
