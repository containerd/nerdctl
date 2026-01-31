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

package container

import (
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunUserName(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.SubTests = []*test.Case{
		{
			Description: "should run Windows container as ContainerAdministrator by default",
			Command:     test.Command("run", "--rm", testutil.WindowsNano, "whoami"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("ContainerAdministrator")),
		},
		{
			Description: "should run Windows container as ContainerAdministrator when user is set to ContainerAdministrator",
			Command:     test.Command("run", "--rm", "--user", "ContainerAdministrator", testutil.WindowsNano, "whoami"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("ContainerAdministrator")),
		},
		{
			Description: "should run Windows container as ContainerUser when user is set to ContainerUser",
			Command:     test.Command("run", "--rm", "--user", "ContainerUser", testutil.WindowsNano, "whoami"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("ContainerUser")),
		},
	}
	testCase.Run(t)
}
