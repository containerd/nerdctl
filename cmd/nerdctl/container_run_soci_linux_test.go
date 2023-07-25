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
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestRunSoci(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	requiresSoci(base)

	//counting initial snapshot mounts
	initialMounts, err := exec.Command("mount").Output()
	if err != nil {
		t.Fatal(err)
	}

	remoteSnapshotsInitialCount := strings.Count(string(initialMounts), "fuse.rawBridge")

	if remoteSnapshotsInitialCount != 0 {
		t.Fatalf("initial mounts count isn't zero")
	}

	//validating `nerdctl --snapshotter=soci run` and `soci rpull` behave the same using mounts
	runOutput := base.Cmd("--snapshotter=soci", "run", "--rm", testutil.FfmpegSociImage).Out()
	base.T.Logf("run output: %s", runOutput)

	actualMounts, err := exec.Command("mount").Output()
	if err != nil {
		t.Fatal(err)
	}
	remoteSnapshotsActualCount := strings.Count(string(actualMounts), "fuse.rawBridge")
	base.T.Logf("number of actual mounts: %v", remoteSnapshotsActualCount)

	rmiOutput := base.Cmd("rmi", testutil.FfmpegSociImage).Out()
	base.T.Logf("rmi output: %s", rmiOutput)

	sociExecutable, err := exec.LookPath("soci")
	if err != nil {
		t.Fatalf("SOCI is not installed.")
	}

	rpullCmd := exec.Command(sociExecutable, []string{"image", "rpull", testutil.FfmpegSociImage}...)

	rpullCmd.Env = os.Environ()

	err = rpullCmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	expectedMounts, err := exec.Command("mount").Output()
	if err != nil {
		t.Fatal(err)
	}

	remoteSnapshotsExpectedCount := strings.Count(string(expectedMounts), "fuse.rawBridge")
	base.T.Logf("number of expected mounts: %v", remoteSnapshotsExpectedCount)

	if remoteSnapshotsExpectedCount != remoteSnapshotsActualCount {
		t.Fatalf("incorrect number of remote snapshots; expected=%d, actual=%d",
			remoteSnapshotsExpectedCount, remoteSnapshotsActualCount)
	}
}

func requiresSoci(base *testutil.Base) {
	info := base.Info()
	for _, p := range info.Plugins.Storage {
		if p == "soci" {
			return
		}
	}
	base.T.Skip("test requires soci")
}
