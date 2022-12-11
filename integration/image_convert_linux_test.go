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

package integration

import (
	"fmt"
	"os/exec"
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
	"gotest.tools/v3/icmd"
)

func TestImageConvertNydus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no windows support yet")
	}

	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
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

	// skip if nydusify is not installed
	if _, err := exec.LookPath("nydusify"); err != nil {
		t.Skip("Nydusify is not installed")
	}

	// setup local docker registry
	registryPort := 15000
	registry := testregistry.NewPlainHTTP(base, registryPort)
	defer registry.Cleanup()

	remoteImage := fmt.Sprintf("%s:%d/nydusd-image:test", registry.IP.String(), registryPort)
	base.Cmd("tag", convertedImage, remoteImage).AssertOK()
	defer base.Cmd("rmi", remoteImage).Run()
	base.Cmd("push", "--insecure-registry", remoteImage).AssertOK()
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
	nydusifyCmd.AssertOK()
}
