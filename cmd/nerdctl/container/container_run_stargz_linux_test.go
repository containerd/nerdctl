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
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestRunStargz(t *testing.T) {
	testutil.DockerIncompatible(t)
	if runtime.GOARCH != "amd64" {
		t.Skip("skipping test as FedoraESGZImage is amd64 only")
	}

	base := testutil.NewBase(t)
	helpers.RequiresStargz(base)
	// if stargz snapshotter is functional, "/.stargz-snapshotter" appears
	base.Cmd("--snapshotter=stargz", "run", "--rm", testutil.FedoraESGZImage, "ls", "/.stargz-snapshotter").AssertOK()
}
