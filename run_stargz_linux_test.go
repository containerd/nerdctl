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

func TestRunStargz(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	requiresStargz(base)
	// if stargz snapshotter is functional, "/.stargz-snapshotter" appears
	base.Cmd("--snapshotter=stargz", "run", "--rm", testutil.FedoraESGZImage, "ls", "/.stargz-snapshotter").AssertOK()
}

func requiresStargz(base *testutil.Base) {
	info := base.Info()
	for _, p := range info.Plugins.Storage {
		if p == "stargz" {
			return
		}
	}
	base.T.Skip("test requires stargz")
}
