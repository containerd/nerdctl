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
	"github.com/containerd/nerdctl/pkg/testutil"
	"os"
	"testing"
)

func TestRunVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	rwDir, err := os.MkdirTemp(t.TempDir(), "rw")
	if err != nil {
		t.Fatal(err)
	}
	roDir, err := os.MkdirTemp(t.TempDir(), "ro")
	if err != nil {
		t.Fatal(err)
	}
	rwVolName := tID + "-rw"
	roVolName := tID + "-ro"
	for _, v := range []string{rwVolName, roVolName} {
		defer base.Cmd("volume", "rm", "-f", v).Run()
		base.Cmd("volume", "create", v).AssertOK()
	}

	containerName := tID
	defer base.Cmd("rm", "-f", containerName).Run()

	base.Cmd("run",
		"-d",
		"--name", containerName,
		"-v", fmt.Sprintf("%s:C:\\mnt1", rwDir),
		"-v", fmt.Sprintf("%s:C:\\mnt2ro", roDir),
		"-v", fmt.Sprintf("%s:C:\\mnt3", rwVolName),
		"-v", fmt.Sprintf("%s:C:\\mnt4ro", roVolName),
		testutil.WindowsNano).AssertOK()
	base.Cmd("container", "inspect", containerName).AssertOK()

}

func TestRunAnonymousVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "-v", "C:\\foo", testutil.WindowsNano).AssertOK()
}

func TestRunCopyingUpInitialContentsOnVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	imageName := testutil.WindowsNano
	volName := testutil.Identifier(t) + "-vol"
	defer base.Cmd("volume", "rm", volName).Run()

	//AnonymousVolume
	base.Cmd("run", "--rm", imageName).AssertOK()
	base.Cmd("run", "-v", "C:\\mnt", "--rm", imageName).AssertOK()

	//NamedVolume should be automatically created
	base.Cmd("run", "-v", volName+""+":C:\\mnt", "--rm", imageName).AssertOK()
}
