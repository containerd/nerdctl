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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestVolumePrune(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	base.Cmd("volume", "prune", "-a", "-f").Run()

	vID := base.Cmd("volume", "create").Out()
	base.Cmd("volume", "create", tID+"-1").AssertOK()
	base.Cmd("volume", "create", tID+"-2").AssertOK()

	base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", tID+"-1"), "--name", tID, testutil.CommonImage).AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()

	base.Cmd("volume", "prune", "-f").AssertOutContains(vID)
	base.Cmd("volume", "prune", "-a", "-f").AssertOutContains(tID + "-2")
	base.Cmd("volume", "ls").AssertOutContains(tID + "-1")
	base.Cmd("volume", "ls").AssertOutNotContains(tID + "-2")

	base.Cmd("rm", "-f", tID).AssertOK()
	base.Cmd("volume", "prune", "-a", "-f").AssertOK()
	base.Cmd("volume", "ls").AssertOutNotContains(tID + "-1")
}
