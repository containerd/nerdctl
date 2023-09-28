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
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestCreateProcessContainer(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("create", "--name", tID, testutil.CommonImage, "echo", "foo").AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()
	base.Cmd("ps", "-a").AssertOutContains("Created")
	base.Cmd("start", tID).AssertOK()
	base.Cmd("logs", tID).AssertOutContains("foo")
}

func TestCreateHyperVContainer(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	if !testutil.HyperVSupported() {
		t.Skip("HyperV is not enabled, skipping test")
	}

	base.Cmd("create", "--isolation", "hyperv", "--name", tID, testutil.CommonImage, "echo", "foo").AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()
	base.Cmd("ps", "-a").AssertOutContains("Created")

	base.Cmd("start", tID).AssertOK()
	// hyperv containers take a few seconds to fire up, the test would fail without the sleep
	// EnsureContainerStarted does not work
	time.Sleep(10 * time.Second)

	base.Cmd("logs", tID).AssertOutContains("foo")
}
