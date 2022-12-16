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
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/sirupsen/logrus"
)

func TestSystemPrune(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	base.Cmd("container", "prune", "-f").AssertOK()
	base.Cmd("network", "prune", "-f").AssertOK()
	base.Cmd("volume", "prune", "-f").AssertOK()
	base.Cmd("image", "prune", "-f", "--all").AssertOK()

	nID := testutil.Identifier(t)
	base.Cmd("network", "create", nID).AssertOK()
	defer base.Cmd("network", "rm", nID).Run()

	vID := testutil.Identifier(t)
	base.Cmd("volume", "create", vID).AssertOK()
	defer base.Cmd("volume", "rm", vID).Run()

	tID := testutil.Identifier(t)
	base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", vID), "--net", nID,
		"--name", tID, testutil.CommonImage).AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()

	base.Cmd("ps", "-a").AssertOutContains(tID)
	base.Cmd("images").AssertOutContains("alpine")

	base.Cmd("system", "prune", "-f", "--volumes", "--all").AssertOK()
	base.Cmd("volume", "ls").AssertNoOut(vID)
	base.Cmd("ps", "-a").AssertNoOut(tID)
	base.Cmd("network", "ls").AssertNoOut(nID)
	base.Cmd("images").AssertNoOut("alpine")

	if testutil.GetTarget() != testutil.Nerdctl {
		t.Skip("test skipped for buildkitd is not available with docker-compatible tests")
	}

	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		t.Fatal(err)
	}
	host, err := buildkitutil.GetBuildkitHost(testutil.Namespace)
	if err != nil {
		t.Fatal(err)
	}

	buildctlArgs := buildkitutil.BuildctlBaseArgs(host)
	buildctlArgs = append(buildctlArgs, "du")
	logrus.Debugf("running %s %v", buildctlBinary, buildctlArgs)
	buildctlCmd := exec.Command(buildctlBinary, buildctlArgs...)
	buildctlCmd.Env = os.Environ()
	stdout := bytes.NewBuffer(nil)
	buildctlCmd.Stdout = stdout
	if err := buildctlCmd.Run(); err != nil {
		t.Fatal(err)
	}
	readAll, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readAll), "Total:\t\t0B") {
		t.Errorf("buildkit cache is not pruned: %s", string(readAll))
	}
}
