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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestDiff(t *testing.T) {
	// It is unclear why this is failing with docker when run in parallel
	// Obviously some other container test is interfering
	if testutil.GetTarget() != testutil.Docker {
		t.Parallel()
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", containerName).Run()

	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "touch /a; touch /bin/b; rm /bin/base64").AssertOK()
	// nerdctl contains more output "C /etc", "A /etc/resolv.conf" unlike docker
	base.Cmd("diff", containerName).AssertOutContainsAll(
		"A /a",
		"C /bin",
		"A /bin/b",
		"D /bin/base64",
	)
	base.Cmd("rm", "-f", containerName).AssertOK()
}
