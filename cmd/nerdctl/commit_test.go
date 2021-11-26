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

func TestCommit(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	switch base.Info().CgroupDriver {
	case "none", "":
		t.Skip("requires cgroup (for pausing)")
	}
	const (
		testContainer = "nerdctl-test-commit"
		testImage     = "nerdctl-test-image"
	)
	defer base.Cmd("rm", "-f", testContainer).Run()
	defer base.Cmd("rmi", testImage).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "infinity").AssertOK()
	base.Cmd("exec", testContainer, "sh", "-euxc", `echo hello-test-commit > /foo`).AssertOK()
	base.Cmd("commit", "-c", `CMD ["cat", "/foo"]`, testContainer, testImage).AssertOK()
	base.Cmd("run", "--rm", testImage).AssertOutExactly("hello-test-commit\n")
}
