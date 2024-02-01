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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestBuild(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutExactly("nerdctl-build-test-string\n")

	// DOCKER_BUILDKIT (v20.10): `Error response from daemon: exporter "docker" could not be found`
	if base.Target == testutil.Nerdctl {
		ignoredImageNamed := imageName + "-" + "ignored"
		outputOpt := fmt.Sprintf("--output=type=docker,name=%s", ignoredImageNamed)
		base.Cmd("build", buildCtx, "-t", imageName, outputOpt).AssertOK()

		base.Cmd("run", "--rm", imageName).AssertOutExactly("nerdctl-build-test-string\n")
		base.Cmd("run", "--rm", ignoredImageNamed).AssertFail()
	}
}

// TestBuildBaseImage tests if an image can be built on the previously built image.
// This isn't currently supported by nerdctl with BuildKit OCI worker.
func TestBuildBaseImage(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()
	imageName2 := imageName + "-2"
	defer base.Cmd("rmi", imageName2).Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN echo hello > /hello
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()

	dockerfile2 := fmt.Sprintf(`FROM %s
RUN echo hello2 > /hello2
CMD ["cat", "/hello2"]
	`, imageName)

	buildCtx2, err := createBuildContext(dockerfile2)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx2)

	base.Cmd("build", "-t", imageName2, buildCtx2).AssertOK()
	base.Cmd("build", buildCtx2, "-t", imageName2).AssertOK()

	base.Cmd("run", "--rm", imageName2).AssertOutExactly("hello2\n")
}

// TestBuildFromContainerd tests if an image can be built on an image pulled by nerdctl.
// This isn't currently supported by nerdctl with BuildKit OCI worker.
func TestBuildFromContainerd(t *testing.T) {
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()
	imageName2 := imageName + "-2"
	defer base.Cmd("rmi", imageName2).Run()

	// FIXME: BuildKit sometimes tries to use base image manifests of platforms that hasn't been
	//        pulled by `nerdctl pull`. This leads to "not found" error for the base image.
	//        To avoid this issue, images shared to BuildKit should always be pulled by manifest
	//        digest or `--all-platforms` needs to be added.
	base.Cmd("pull", "--all-platforms", testutil.CommonImage).AssertOK()
	base.Cmd("tag", testutil.CommonImage, imageName).AssertOK()
	base.Cmd("rmi", testutil.CommonImage).AssertOK()

	dockerfile2 := fmt.Sprintf(`FROM %s
RUN echo hello2 > /hello2
CMD ["cat", "/hello2"]
	`, imageName)

	buildCtx2, err := createBuildContext(dockerfile2)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx2)

	base.Cmd("build", "-t", imageName2, buildCtx2).AssertOK()
	base.Cmd("build", buildCtx2, "-t", imageName2).AssertOK()

	base.Cmd("run", "--rm", imageName2).AssertOutExactly("hello2\n")
}

func TestBuildFromStdin(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-stdin"]
	`, testutil.CommonImage)

	base.Cmd("build", "-t", imageName, "-f", "-", ".").CmdOption(testutil.WithStdin(strings.NewReader(dockerfile))).AssertCombinedOutContains(imageName)
}

func TestBuildWithDockerfile(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-dockerfile"]
	`, testutil.CommonImage)

	buildCtx := filepath.Join(t.TempDir(), "test")
	err := os.MkdirAll(buildCtx, 0755)
	assert.NilError(t, err)
	err = os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0644)
	assert.NilError(t, err)

	pwd, err := os.Getwd()
	assert.NilError(t, err)
	err = os.Chdir(buildCtx)
	assert.NilError(t, err)
	defer os.Chdir(pwd)

	// hack os.Getwd return "(unreachable)" on rootless
	t.Setenv("PWD", buildCtx)

	base.Cmd("build", "-t", imageName, "-f", "Dockerfile", "..").AssertOK()
	base.Cmd("build", "-t", imageName, "-f", "Dockerfile", ".").AssertOK()
	// fail err: no such file or directory
	base.Cmd("build", "-t", imageName, "-f", "../Dockerfile", ".").AssertFail()
}

func TestBuildLocal(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	if testutil.GetTarget() == testutil.Docker {
		base.Env = append(base.Env, "DOCKER_BUILDKIT=1")
	}
	defer base.Cmd("builder", "prune").Run()
	const testFileName = "nerdctl-build-test"
	const testContent = "nerdctl"
	outputDir := t.TempDir()

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

	aliasOutputDir := t.TempDir()
	testAliasFilePath := filepath.Join(aliasOutputDir, testFileName)
	base.Cmd("build", "-o", aliasOutputDir, buildCtx).AssertOK()
	if _, err := os.Stat(testAliasFilePath); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(testAliasFilePath)
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

func TestBuildWithBuildArg(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
ARG TEST_STRING=1
ENV TEST_STRING=$TEST_STRING
CMD echo $TEST_STRING
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutExactly("1\n")

	validCases := []struct {
		name     string
		arg      string
		envValue string
		envSet   bool
		expected string
	}{
		{"ArgValueOverridesDefault", "TEST_STRING=2", "", false, "2\n"},
		{"EmptyArgValueOverridesDefault", "TEST_STRING=", "", false, "\n"},
		{"UnsetArgKeyPreservesDefault", "TEST_STRING", "", false, "1\n"},
		{"EnvValueOverridesDefault", "TEST_STRING", "3", true, "3\n"},
		{"EmptyEnvValueOverridesDefault", "TEST_STRING", "", true, "\n"},
	}

	for _, tc := range validCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envSet {
				err := os.Setenv("TEST_STRING", tc.envValue)
				assert.NilError(t, err)
				defer os.Unsetenv("TEST_STRING")
			}

			base.Cmd("build", buildCtx, "-t", imageName, "--build-arg", tc.arg).AssertOK()
			base.Cmd("run", "--rm", imageName).AssertOutExactly(tc.expected)
		})
	}
}

func TestBuildWithIIDFile(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)
	fileName := filepath.Join(t.TempDir(), "id.txt")

	base.Cmd("build", "-t", imageName, buildCtx, "--iidfile", fileName).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName, "--iidfile", fileName).AssertOK()
	defer os.Remove(fileName)

	imageID, err := os.ReadFile(fileName)
	assert.NilError(t, err)

	base.Cmd("run", "--rm", string(imageID)).AssertOutExactly("nerdctl-build-test-string\n")
}

func TestBuildWithLabels(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)

	dockerfile := fmt.Sprintf(`FROM %s
LABEL name=nerdctl-build-test-label
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx, "--label", "label=test").AssertOK()
	defer base.Cmd("rmi", imageName).Run()

	base.Cmd("inspect", imageName, "--format", "{{json .Config.Labels }}").AssertOutExactly("{\"label\":\"test\",\"name\":\"nerdctl-build-test-label\"}\n")
}

func TestBuildMultipleTags(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	img := testutil.Identifier(t)
	imgWithNoTag, imgWithCustomTag := fmt.Sprintf("%s%d", img, 2), fmt.Sprintf("%s%d:hello", img, 3)
	defer base.Cmd("rmi", img).AssertOK()
	defer base.Cmd("rmi", imgWithNoTag).AssertOK()
	defer base.Cmd("rmi", imgWithCustomTag).AssertOK()

	buildOutputs := []string{"nerdctl-build-test-string", "nerdctl-build-test-string-rebuild"}
	for _, output := range buildOutputs {
		dockerfile := fmt.Sprintf(`FROM %s
		CMD ["echo", "%s"]
			`, testutil.CommonImage, output)
		buildCtx, err := createBuildContext(dockerfile)
		assert.NilError(t, err)
		defer os.RemoveAll(buildCtx)

		base.Cmd("build", "-t", img, buildCtx).AssertOK()
		base.Cmd("build", buildCtx, "-t", img, "-t", imgWithNoTag, "-t", imgWithCustomTag).AssertOK()
		base.Cmd("run", "--rm", img).AssertOutExactly(output + "\n")
		base.Cmd("run", "--rm", imgWithNoTag).AssertOutExactly(output + "\n")
		base.Cmd("run", "--rm", imgWithCustomTag).AssertOutExactly(output + "\n")
	}
}

func TestBuildWithContainerfile(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	containerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx := t.TempDir()

	var err = os.WriteFile(filepath.Join(buildCtx, "Containerfile"), []byte(containerfile), 0644)
	assert.NilError(t, err)
	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutExactly("nerdctl-build-test-string\n")
}

func TestBuildWithDockerFileAndContainerfile(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "dockerfile"]
	`, testutil.CommonImage)

	containerfile := fmt.Sprintf(`FROM %s
	CMD ["echo", "containerfile"]
		`, testutil.CommonImage)

	tmpDir := t.TempDir()

	var err = os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644)
	assert.NilError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "Containerfile"), []byte(containerfile), 0644)
	assert.NilError(t, err)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutExactly("dockerfile\n")
}

func TestBuildNoTag(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").AssertOK()
	base.Cmd("image", "prune", "--force", "--all").AssertOK()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-notag-string"]
	`, testutil.CommonImage)
	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", buildCtx).AssertOK()
	base.Cmd("images").AssertOutContains("<none>")
	base.Cmd("image", "prune", "--force", "--all").AssertOK()
}

// TestBuildSourceDateEpoch tests that $SOURCE_DATE_EPOCH is propagated from the client env
// https://github.com/docker/buildx/pull/1482
func TestBuildSourceDateEpoch(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.DockerIncompatible(t) // Needs buildx v0.10 (https://github.com/docker/buildx/pull/1489)
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).AssertOK()

	dockerfile := fmt.Sprintf(`FROM %s
ARG SOURCE_DATE_EPOCH
RUN echo $SOURCE_DATE_EPOCH >/source-date-epoch
CMD ["cat", "/source-date-epoch"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	const sourceDateEpochEnvStr = "1111111111"
	t.Setenv("SOURCE_DATE_EPOCH", sourceDateEpochEnvStr)
	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutExactly(sourceDateEpochEnvStr + "\n")

	const sourceDateEpochArgStr = "2222222222"
	base.Cmd("build", "-t", imageName, "--build-arg", "SOURCE_DATE_EPOCH="+sourceDateEpochArgStr, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutExactly(sourceDateEpochArgStr + "\n")
}

func TestBuildNetwork(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").AssertOK()

	dockerfile := fmt.Sprintf(`FROM %s
RUN apk add --no-cache curl
RUN curl -I http://google.com
	`, testutil.CommonImage)
	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	validCases := []struct {
		name     string
		network  string
		exitCode int
	}{
		// When network=none, can't connect to internet, therefore cannot download packages in the dockerfile
		// Order is important here, test fails for `-test.target=docker` in CI
		{"test_with_no_network", "none", 1},
		{"test_with_empty_network", "", 0},
		{"test_with_default_network", "default", 0},
	}

	for _, tc := range validCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// --no-cache is intentional here for `-test.target=docker`
			base.Cmd("build", buildCtx, "-t", tc.name, "--no-cache", "--network", tc.network).AssertExitCode(tc.exitCode)
			if tc.exitCode != 1 {
				defer base.Cmd("rmi", tc.name).AssertOK()
			}
		})
	}
}

func TestBuildNetworkShellCompletion(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	const gsc = "__complete"
	// Tests with build network
	networkName := "default"
	base.Cmd(gsc, "build", "--network", "").AssertOutContains(networkName)
}

func buildWithNamedBuilder(base *testutil.Base, builderName string, args ...string) *testutil.Cmd {
	buildArgs := []string{"build"}
	if testutil.GetTarget() == testutil.Docker {
		buildArgs = append(buildArgs, "--builder", builderName)
	}
	buildArgs = append(buildArgs, args...)
	return base.Cmd(buildArgs...)
}

func TestBuildAttestation(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	builderName := testutil.Identifier(t)
	if testutil.GetTarget() == testutil.Docker {
		// create named builder for docker
		defer base.Cmd("buildx", "rm", builderName).AssertOK()
		base.Cmd("buildx", "create", "--name", builderName, "--bootstrap", "--use").AssertOK()
	}
	defer base.Cmd("builder", "prune").Run()

	dockerfile := "FROM " + testutil.NginxAlpineImage
	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	// Test sbom
	outputSBOMDir := t.TempDir()
	buildWithNamedBuilder(base, builderName, "--sbom=true", "-o", fmt.Sprintf("type=local,dest=%s", outputSBOMDir), buildCtx).AssertOK()
	const testSBOMFileName = "sbom.spdx.json"
	testSBOMFilePath := filepath.Join(outputSBOMDir, testSBOMFileName)
	if _, err := os.Stat(testSBOMFilePath); err != nil {
		t.Fatal(err)
	}

	// Test provenance
	outputProvenanceDir := t.TempDir()
	buildWithNamedBuilder(base, builderName, "--provenance=mode=min", "-o", fmt.Sprintf("type=local,dest=%s", outputProvenanceDir), buildCtx).AssertOK()
	const testProvenanceFileName = "provenance.json"
	testProvenanceFilePath := filepath.Join(outputProvenanceDir, testProvenanceFileName)
	if _, err := os.Stat(testProvenanceFilePath); err != nil {
		t.Fatal(err)
	}

	// Test attestation
	outputAttestationDir := t.TempDir()
	buildWithNamedBuilder(base, builderName, "--attest=type=provenance,mode=min", "--attest=type=sbom", "-o", fmt.Sprintf("type=local,dest=%s", outputAttestationDir), buildCtx).AssertOK()
	testSBOMFilePath = filepath.Join(outputAttestationDir, testSBOMFileName)
	testProvenanceFilePath = filepath.Join(outputAttestationDir, testProvenanceFileName)
	if _, err := os.Stat(testSBOMFilePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(testProvenanceFilePath); err != nil {
		t.Fatal(err)
	}
}
