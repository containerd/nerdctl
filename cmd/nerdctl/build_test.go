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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestBuild(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const imageName = "nerdctl-build-test"
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()

	base.Cmd("run", "--rm", imageName).AssertOutContains("nerdctl-build-test-string")
}

func TestBuildFromStdin(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const imageName = "nerdctl-build-stdin-test"
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-stdin"]
	`, testutil.AlpineImage)

	base.Cmd("build", "-t", imageName, "-f", "-", ".").CmdOption(testutil.WithStdin(strings.NewReader(dockerfile))).AssertOutContains("nerdctl-build-stdin-test")
}

func TestBuildLocal(t *testing.T) {
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const testFileName = "nerdctl-build-test"
	const testContent = "nerdctl"
	outputDir, err := os.MkdirTemp("", "nerdctl-build-test-")
	assert.NilError(t, err)
	defer os.RemoveAll(outputDir)

	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /`,
		testFileName)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	if err := os.WriteFile(filepath.Join(buildCtx, testFileName), []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	testFilePath := filepath.Join(outputDir, testFileName)
	base.Cmd("build", "-o", fmt.Sprintf("type=local,dest=%s", outputDir), buildCtx).AssertOK()
	if _, err := os.Stat(testFilePath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(testFilePath)
	assert.NilError(t, err)
	assert.Equal(t, string(data), testContent)
}

func createBuildContext(dockerfile string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "nerdctl-build-test")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return "", err
	}
	return tmpDir, nil
}
