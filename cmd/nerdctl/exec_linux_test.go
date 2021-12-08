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

func TestExecWithUser(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	const testContainer = "nerdctl-test-exec"
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "infinity").AssertOK()

	testCases := map[string]string{
		"":             "uid=0(root) gid=0(root)",
		"1000":         "uid=1000 gid=0(root)",
		"1000:users":   "uid=1000 gid=100(users)",
		"guest":        "uid=405(guest) gid=100(users)",
		"nobody":       "uid=65534(nobody) gid=65534(nobody)",
		"nobody:users": "uid=65534(nobody) gid=100(users)",
	}

	for userStr, expected := range testCases {
		cmd := []string{"exec"}
		if userStr != "" {
			cmd = append(cmd, "--user", userStr)
		}
		cmd = append(cmd, testContainer, "id")
		base.Cmd(cmd...).AssertOutContains(expected)
	}
}
