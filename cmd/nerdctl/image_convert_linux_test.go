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

	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

func TestImageConvertNydus(t *testing.T) {
	testutil.RequireExecutable(t, "nydus-image")
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	t.Parallel()

	convertedImage := testutil.Identifier(t) + ":nydus"
	base.Cmd("rmi", convertedImage).Run()
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	base.Cmd("image", "convert", "--nydus", "--oci",
		testutil.CommonImage, convertedImage).AssertOK()
	defer base.Cmd("rmi", convertedImage).Run()

	// use `nydusify` check whether the convertd nydus image is valid

	// skip if rootless
	if rootlessutil.IsRootless() {
		t.Skip("Nydusify check is not supported rootless mode.")
	}

	// skip if nydusify and nydusd are not installed
	testutil.RequireExecutable(t, "nydusify")
	testutil.RequireExecutable(t, "nydusd")

	// setup local docker registry
	registry := testregistry.NewWithNoAuth(base, 0, false)
	remoteImage := fmt.Sprintf("%s:%d/nydusd-image:test", "localhost", registry.Port)
	t.Cleanup(func() {
		base.Cmd("rmi", remoteImage).Run()
		registry.Cleanup(nil)
	})

	base.Cmd("tag", convertedImage, remoteImage).AssertOK()
	base.Cmd("push", remoteImage).AssertOK()
	nydusifyCmd := testutil.Cmd{
		Cmd: icmd.Command(
			"nydusify",
			"check",
			"--source",
			testutil.CommonImage,
			"--target",
			remoteImage,
			"--source-insecure",
			"--target-insecure",
		),
		Base: base,
	}

	// nydus is creating temporary files - make sure we are in a proper location for that
	nydusifyCmd.Cmd.Dir = base.T.TempDir()
	nydusifyCmd.AssertOK()
}
