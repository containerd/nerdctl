/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

// TestIssue108 tests https://github.com/containerd/nerdctl/issues/108
// ("`nerdctl run --net=host -it` fails while `nerdctl run -it --net=host` works")
func TestIssue108(t *testing.T) {
	base := testutil.NewBase(t)
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.CmdWithHelper(unbuffer, "run", "-it", "--rm", "--net=host", testutil.AlpineImage,
		"echo", "this was always working").AssertOK()
	base.CmdWithHelper(unbuffer, "run", "--rm", "--net=host", "-it", testutil.AlpineImage,
		"echo", "this was not working due to issue #108").AssertOK()
}
