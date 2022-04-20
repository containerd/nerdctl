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

func TestContainerPrune(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "testContainer1",
			command: "create",
		},
		{
			name:    "testContainer2",
			command: "run",
		},
	}
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	for _, test := range tests {
		base.Cmd(test.command, "--name", test.name+tID, testutil.CommonImage).AssertOK()

	}
	defer base.Cmd("container", "prune", "-f").Run()
}
