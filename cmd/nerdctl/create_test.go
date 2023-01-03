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
	"os"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

// TestCreateDuplicateName When the container's name file is wrong(such as being deleted),
// avoid creating containers with the same name
func TestCreateDuplicateName(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	imageName := testutil.CommonImage
	containerName := testutil.Identifier(t)

	base.Cmd("create", "--name", containerName, imageName).AssertOK()
	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()

	store := strings.Split(base.InspectContainer(containerName).ResolvConfPath, "/containers/")
	if len(store) != 2 {
		t.Fatalf("parse store path fail")
	}
	// store[0]
	// root: /var/lib/nerdctl/1935db59
	// rootless: ~/.local/share/nerdctl/1935db59
	fileName := fmt.Sprintf("%s/names/%s/%s", store[0], testutil.Namespace, containerName)
	_, err := os.Stat(fileName)
	assert.NilError(t, err)
	err = os.Remove(fileName)
	assert.NilError(t, err)

	base.Cmd("create", "--name", containerName, imageName).AssertFail()
}
