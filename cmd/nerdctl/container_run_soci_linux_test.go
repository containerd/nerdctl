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
	"os/exec"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestRunSoci(t *testing.T) {
	testutil.DockerIncompatible(t)
	tests := []struct {
		name                         string
		image                        string
		remoteSnapshotsExpectedCount int
	}{
		{
			name:                         "Run with SOCI",
			image:                        testutil.FfmpegSociImage,
			remoteSnapshotsExpectedCount: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := testutil.NewBase(t)
			helpers.RequiresSoci(base)

			//counting initial snapshot mounts
			initialMounts, err := exec.Command("mount").Output()
			if err != nil {
				t.Fatal(err)
			}

			remoteSnapshotsInitialCount := strings.Count(string(initialMounts), "fuse.rawBridge")

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

			base.T.Logf("number of expected mounts: %v", tt.remoteSnapshotsExpectedCount)

			if tt.remoteSnapshotsExpectedCount != (remoteSnapshotsActualCount - remoteSnapshotsInitialCount) {
				t.Fatalf("incorrect number of remote snapshots; expected=%d, actual=%d",
					tt.remoteSnapshotsExpectedCount, remoteSnapshotsActualCount-remoteSnapshotsInitialCount)
			}
		})
	}
}
