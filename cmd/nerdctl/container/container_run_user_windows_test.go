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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestRunUserName(t *testing.T) {
	base := testutil.NewBase(t)
	testCases := []struct {
		explicitUser string
		whoami       string
		env          string
	}{
		{
			explicitUser: "",
			whoami:       "root",
			env:          "ContainerAdministrator",
		},
		{
			explicitUser: "ContainerUser",
			whoami:       "ContainerUser",
			env:          "ContainerUser",
		},
		{
			explicitUser: "ContainerAdministrator",
			whoami:       "root",
			env:          "ContainerAdministrator",
		},
	}

	for _, user := range testCases {
		t.Run(user.explicitUser, func(t *testing.T) {
			t.Parallel()
			cmd := []string{"run", "--rm"}
			if user.explicitUser != "" {
				cmd = append(cmd, "--user", user.explicitUser)
			}
			cmd = append(cmd, testutil.WindowsNano, "whoami")
			base.Cmd(cmd...).AssertOutContains(user.whoami)

			cmd = append(cmd, testutil.WindowsNano, "echo $USERNAME")
			base.Cmd(cmd...).AssertOutContains(user.whoami)
		})
	}
}
