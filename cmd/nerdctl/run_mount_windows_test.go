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
	"strings"
	"testing"
)

func TestRunVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	imageName := testutil.WindowsNano
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
		imageName).AssertOK()
	base.Cmd("container", "inspect", containerName).AssertOK()

}

func TestRunAnonymousVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	imageName := testutil.WindowsNano

	base.Cmd("run", "--rm", imageName).AssertOK()
	aop := base.Cmd("run", "-d", "-v", "C:\\mnt", imageName).OutLines()
	defer base.Cmd("rm", aop[0]).Run()
	cont := base.InspectContainer(aop[0])
	if cont.Mounts != nil {
		fmt.Println("Anonymous Volume Mounted")
	} else {
		t.Fail()
	}
}

func TestRunNamedVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	imageName := testutil.WindowsNano
	volName := testutil.Identifier(t) + "-vol"
	defer base.Cmd("volume", "rm", volName).Run()

	//NamedVolume should be automatically created
	op := base.Cmd("run", "-d", "-v", volName+":C:\\mnt", imageName).OutLines()
	defer base.Cmd("rm", op[0]).Run()
	cont := base.InspectContainer(op[0])
	if cont.Mounts != nil {
		src := strings.Split(cont.Mounts[0].Source, "\\")
		if volName == src[len(src)-2] {
			fmt.Println("Named Volume Mounted")
		}
	} else {
		t.Fail()
	}
}
