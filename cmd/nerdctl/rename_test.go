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
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestRename(t *testing.T) {
	tests := []struct {
		containerName    string
		containerNewName string
		wantFail         bool
	}{
		{
			containerName:    "testContainer0",
			containerNewName: "testContainerNew0",
			wantFail:         false,
		},
		{
			containerName:    "testContainer1",
			containerNewName: "testContainerNew1",
			wantFail:         false,
		},
		{
			containerName:    "testContainerSame",
			containerNewName: "testContainerSame",
			wantFail:         true,
		},
	}
	t.Parallel()
	base := testutil.NewBase(t)
	imageName := testutil.CommonImage
	for _, test := range tests {
		id := base.Cmd("run", "-d", "--name", test.containerName, imageName).OutLines()

		result := base.Cmd("rename", test.containerName, test.containerNewName).Run()
		if result.Error != nil {
			if test.wantFail {
				fmt.Println(test.containerName, "container cannot be renamed to", test.containerNewName, ": Renaming a container with the same name as its current name")
			}
		} else {
			cont := base.InspectContainer(id[0])
			if test.containerNewName == cont.Name {
				fmt.Println(test.containerName, "Container Renamed to", test.containerNewName)
			}
		}
		base.Cmd("rm", test.containerNewName).AssertOK()
	}
}
