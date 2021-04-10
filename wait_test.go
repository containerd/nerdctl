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

func TestWait(t *testing.T) {
	const (
		testContainerName1 = "nerdctl-test-wait-1"
		testContainerName2 = "nerdctl-test-wait-2"
		testContainerName3 = "nerdctl-test-wait-3"
	)

	const expected = `0
0
123
`
	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainerName1, testContainerName2, testContainerName3).Run()

	base.Cmd("run", "-d", "--name", testContainerName1, testutil.AlpineImage, "sleep", "2").AssertOK()

	base.Cmd("run", "-d", "--name", testContainerName2, testutil.AlpineImage, "sleep", "2").AssertOK()

	base.Cmd("run", "--name", testContainerName3, testutil.AlpineImage, "sh", "-euxc", "sleep 3; exit 123").AssertExitCode(123)

	base.Cmd("wait", testContainerName1, testContainerName2, testContainerName3).AssertOut(expected)

}
