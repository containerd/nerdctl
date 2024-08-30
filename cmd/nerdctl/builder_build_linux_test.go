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

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestBuildContextWithOCILayout(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)

	var dockerBuilderArgs []string
	if testutil.IsDocker() {
		// Default docker driver does not support OCI exporter.
		// Reference: https://docs.docker.com/build/exporters/oci-docker/
		builderName := testutil.SetupDockerContainerBuilder(t)
		dockerBuilderArgs = []string{"buildx", "--builder", builderName}
	}

	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	ociLayout := "parent"
	parentImageName := fmt.Sprintf("%s-%s", imageName, ociLayout)

	teardown := func() {
		base.Cmd("rmi", parentImageName, imageName).Run()
	}
	t.Cleanup(teardown)
	teardown()

	dockerfile := fmt.Sprintf(`FROM %s
LABEL layer=oci-layout-parent
CMD ["echo", "test-nerdctl-build-context-oci-layout-parent"]`, testutil.CommonImage)
	buildCtx := helpers.CreateBuildContext(t, dockerfile)

	tarPath := fmt.Sprintf("%s/%s.tar", buildCtx, ociLayout)

	// Create OCI archive from parent image.
	base.Cmd("build", buildCtx, "--tag", parentImageName).AssertOK()
	base.Cmd("image", "save", "--output", tarPath, parentImageName).AssertOK()

	// Unpack OCI archive into OCI layout directory.
	ociLayoutDir := t.TempDir()
	err := extractTarFile(ociLayoutDir, tarPath)
	assert.NilError(t, err)

	dockerfile = fmt.Sprintf(`FROM %s
CMD ["echo", "test-nerdctl-build-context-oci-layout"]`, ociLayout)
	buildCtx = helpers.CreateBuildContext(t, dockerfile)

	var buildArgs = []string{}
	if testutil.IsDocker() {
		buildArgs = dockerBuilderArgs
	}

	buildArgs = append(buildArgs, "build", buildCtx, fmt.Sprintf("--build-context=%s=oci-layout://%s", ociLayout, ociLayoutDir), "--tag", imageName)
	if testutil.IsDocker() {
		// Need to load the container image from the builder to be able to run it.
		buildArgs = append(buildArgs, "--load")
	}

	base.Cmd(buildArgs...).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutContains("test-nerdctl-build-context-oci-layout")
}
