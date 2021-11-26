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

func TestMain(m *testing.M) {
	testutil.M(m)
}

// TestUnknownCommand tests https://github.com/containerd/nerdctl/issues/487
func TestUnknownCommand(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("non-existent-command").AssertFail()
	base.Cmd("non-existent-command", "info").AssertFail()
	base.Cmd("system", "non-existent-command").AssertFail()
	base.Cmd("system", "non-existent-command", "info").AssertFail()
	base.Cmd("system").AssertOK() // show help without error
	base.Cmd("system", "info").AssertOutContains("Kernel Version:")
	base.Cmd("info").AssertOutContains("Kernel Version:")
}
