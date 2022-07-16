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

func TestRemoveImage(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	base.Cmd("image", "prune", "--force", "--all").AssertOK()

	// ignore error
	base.Cmd("rmi", "-f", tID).AssertOK()

	base.Cmd("run", "--name", tID, testutil.CommonImage).AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()

	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	defer base.Cmd("rmi", "-f", testutil.CommonImage).Run()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()

	base.Cmd("images").AssertNoOut(testutil.CommonImage)
}
