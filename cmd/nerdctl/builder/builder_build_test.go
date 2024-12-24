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

package builder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestBuildBasics(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]`, testutil.CommonImage)
			err := os.WriteFile(filepath.Join(data.TempDir(), "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", data.TempDir())
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		SubTests: []*test.Case{
			{
				Description: "Successfully build with 'tag first', 'buildctx second'",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", "-t", data.Identifier(), data.Get("buildCtx"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "Successfully build with 'buildctx first', 'tag second'",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "Successfully build with output docker, main tag still works",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "-t", data.Identifier(), "--output=type=docker,name="+data.Identifier("ignored"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "Successfully build with output docker, name cannot be used",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "-t", data.Identifier(), "--output=type=docker,name="+data.Identifier("ignored"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier("ignored"))
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(-1, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestCanBuildOnOtherPlatform(t *testing.T) {
	nerdtest.Setup()

	requireEmulation := &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers) (bool, string) {
			candidateArch := "arm64"
			if runtime.GOARCH == "arm64" {
				candidateArch = "amd64"
			}
			can, err := platformutil.CanExecProbably("linux/" + candidateArch)
			assert.NilError(helpers.T(), err)

			data.Set("OS", "linux")
			data.Set("Architecture", candidateArch)
			return can, "Current environment does not support emulation"
		},
	}

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			requireEmulation,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
RUN echo hello > /hello
CMD ["echo", "nerdctl-build-test-string"]`, testutil.CommonImage)
			err := os.WriteFile(filepath.Join(data.TempDir(), "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", data.TempDir())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("build", data.Get("buildCtx"), "--platform", fmt.Sprintf("%s/%s", data.Get("OS"), data.Get("Architecture")), "-t", data.Identifier())
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}

// TestBuildBaseImage tests if an image can be built on the previously built image.
// This isn't currently supported by nerdctl with BuildKit OCI worker.
func TestBuildBaseImage(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier("first"))
			helpers.Anyhow("rmi", "-f", data.Identifier("second"))
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
RUN echo hello > /hello
CMD ["echo", "nerdctl-build-test-string"]`, testutil.CommonImage)
			err := os.WriteFile(filepath.Join(data.TempDir(), "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", "-t", data.Identifier("first"), data.TempDir())

			dockerfileSecond := fmt.Sprintf(`FROM %s
RUN echo hello2 > /hello2
CMD ["cat", "/hello2"]`, data.Identifier("first"))
			err = os.WriteFile(filepath.Join(data.TempDir(), "Dockerfile"), []byte(dockerfileSecond), 0644)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", "-t", data.Identifier("second"), data.TempDir())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier("second"))
		},
		Expected: test.Expects(0, nil, test.Equals("hello2\n")),
	}

	testCase.Run(t)
}

// TestBuildFromContainerd tests if an image can be built on an image pulled by nerdctl.
// This isn't currently supported by nerdctl with BuildKit OCI worker.
func TestBuildFromContainerd(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			test.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier("first"))
			helpers.Anyhow("rmi", "-f", data.Identifier("second"))
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			helpers.Ensure("tag", testutil.CommonImage, data.Identifier("first"))

			dockerfile := fmt.Sprintf(`FROM %s
RUN echo hello2 > /hello2
CMD ["cat", "/hello2"]`, data.Identifier("first"))
			err := os.WriteFile(filepath.Join(data.TempDir(), "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", "-t", data.Identifier("second"), data.TempDir())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier("second"))
		},
		Expected: test.Expects(0, nil, test.Equals("hello2\n")),
	}

	testCase.Run(t)
}

func TestBuildFromStdin(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-stdin"]`, testutil.CommonImage)
			cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "-", ".")
			cmd.WithStdin(strings.NewReader(dockerfile))
			return cmd
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Errors: []error{errors.New(data.Identifier())},
			}
		},
	}

	testCase.Run(t)
}

func TestBuildWithDockerfile(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-dockerfile"]
	`, testutil.CommonImage)
			buildCtx := filepath.Join(data.TempDir(), "test")
			err := os.MkdirAll(buildCtx, 0755)
			assert.NilError(helpers.T(), err)
			err = os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
		},
		SubTests: []*test.Case{
			{
				Description: "Dockerfile ..",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "Dockerfile", "..")
					cmd.WithCwd(data.Get("buildCtx"))
					return cmd
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Dockerfile .",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "Dockerfile", ".")
					cmd.WithCwd(data.Get("buildCtx"))
					return cmd
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "../Dockerfile .",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "../Dockerfile", ".")
					cmd.WithCwd(data.Get("buildCtx"))
					return cmd
				},
				Expected: test.Expects(1, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildLocal(t *testing.T) {
	nerdtest.Setup()

	const testFileName = "nerdctl-build-test"
	const testContent = "nerdctl"

	testCase := &test.Case{
		Require: nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /`, testFileName)

			err := os.WriteFile(filepath.Join(data.TempDir(), "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)

			err = os.WriteFile(filepath.Join(data.TempDir(), testFileName), []byte(testContent), 0644)
			assert.NilError(helpers.T(), err)

			data.Set("buildCtx", data.TempDir())
		},
		SubTests: []*test.Case{
			{
				Description: "destination 1",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", "-o", fmt.Sprintf("type=local,dest=%s", data.TempDir()), data.Get("buildCtx"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							testFilePath := filepath.Join(data.TempDir(), testFileName)
							_, err := os.Stat(testFilePath)
							assert.NilError(helpers.T(), err, info)
							dt, err := os.ReadFile(testFilePath)
							assert.NilError(helpers.T(), err, info)
							assert.Equal(helpers.T(), string(dt), testContent, info)
						},
					}
				},
			},
			{
				Description: "destination 2",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", "-o", data.TempDir(), data.Get("buildCtx"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							testFilePath := filepath.Join(data.TempDir(), testFileName)
							_, err := os.Stat(testFilePath)
							assert.NilError(helpers.T(), err, info)
							dt, err := os.ReadFile(testFilePath)
							assert.NilError(helpers.T(), err, info)
							assert.Equal(helpers.T(), string(dt), testContent, info)
						},
					}
				},
			},
		},
	}

	testCase.Run(t)
}

func TestBuildWithBuildArg(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
ARG TEST_STRING=1
ENV TEST_STRING=$TEST_STRING
CMD echo $TEST_STRING
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
		},
		SubTests: []*test.Case{
			{
				Description: "No args",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("1\n")),
			},
			{
				Description: "ArgValueOverridesDefault",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "--build-arg", "TEST_STRING=2", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("2\n")),
			},
			{
				Description: "EmptyArgValueOverridesDefault",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "--build-arg", "TEST_STRING=", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("\n")),
			},
			{
				Description: "UnsetArgKeyPreservesDefault",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "--build-arg", "TEST_STRING", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("1\n")),
			},
			{
				Description: "EnvValueOverridesDefault",
				Env: map[string]string{
					"TEST_STRING": "3",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "--build-arg", "TEST_STRING", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("3\n")),
			},
			{
				Description: "EmptyEnvValueOverridesDefault",
				Env: map[string]string{
					"TEST_STRING": "",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "--build-arg", "TEST_STRING", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("\n")),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildWithIIDFile(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", buildCtx, "--iidfile", filepath.Join(data.TempDir(), "id.txt"), "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			imageID, err := os.ReadFile(filepath.Join(data.TempDir(), "id.txt"))
			assert.NilError(helpers.T(), err)
			return helpers.Command("run", "--rm", string(imageID))
		},

		Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
	}

	testCase.Run(t)
}

func TestBuildWithLabels(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
LABEL name=nerdctl-build-test-label
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", buildCtx, "--label", "label=test", "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("inspect", data.Identifier(), "--format", "{{json .Config.Labels }}")
		},

		Expected: test.Expects(0, nil, test.Equals("{\"label\":\"test\",\"name\":\"nerdctl-build-test-label\"}\n")),
	}

	testCase.Run(t)
}

func TestBuildMultipleTags(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Data: test.WithData("i1", "image").
			Set("i2", "image2").
			Set("i3", "image3:hello"),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Get("i1"))
			helpers.Anyhow("rmi", "-f", data.Get("i2"))
			helpers.Anyhow("rmi", "-f", data.Get("i3"))
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", buildCtx, "-t", data.Get("i1"), "-t", data.Get("i2"), "-t", data.Get("i3"))
		},
		SubTests: []*test.Case{
			{
				Description: "i1",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Get("i1"))
				},

				Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "i2",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Get("i2"))
				},

				Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "i3",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Get("i3"))
				},

				Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildWithContainerfile(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			test.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Containerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", buildCtx, "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier())
		},
		Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
	}

	testCase.Run(t)
}

func TestBuildWithDockerFileAndContainerfile(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "dockerfile"]
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			dockerfile = fmt.Sprintf(`FROM %s
CMD ["echo", "containerfile"]
	`, testutil.CommonImage)
			err = os.WriteFile(filepath.Join(buildCtx, "Containerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", buildCtx, "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier())
		},
		Expected: test.Expects(0, nil, test.Equals("dockerfile\n")),
	}

	testCase.Run(t)
}

func TestBuildNoTag(t *testing.T) {
	nerdtest.Setup()

	// FIXME: this test should be rewritten and instead get the image id from the build, then query the image explicitly - instead of pruning / noparallel
	testCase := &test.Case{
		NoParallel: true,
		Require:    nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("image", "prune", "--force", "--all")
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			helpers.Ensure("build", buildCtx)
		},
		Command:  test.Command("images"),
		Expected: test.Expects(0, nil, test.Contains("<none>")),
	}

	testCase.Run(t)
}

func TestBuildContextDockerImageAlias(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := `FROM myorg/myapp
CMD ["echo", "nerdctl-build-myorg/myapp"]`
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("build", "-t", data.Identifier(), data.Get("buildCtx"), fmt.Sprintf("--build-context=myorg/myapp=docker-image://%s", testutil.CommonImage))
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}

func TestBuildContextWithCopyFromDir(t *testing.T) {
	nerdtest.Setup()

	content := "hello_from_dir_2"
	filename := "hello.txt"

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			test.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dir2 := helpers.T().TempDir()
			filePath := filepath.Join(dir2, filename)
			err := os.WriteFile(filePath, []byte(content), 0o600)
			assert.NilError(helpers.T(), err)
			dockerfile := fmt.Sprintf(`FROM %s
COPY --from=dir2 /%s /hello_from_dir2.txt
RUN ["cat", "/hello_from_dir2.txt"]`, testutil.CommonImage, filename)
			buildCtx := data.TempDir()
			err = os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
			data.Set("dir2", dir2)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("build", "-t", data.Identifier(), data.Get("buildCtx"), fmt.Sprintf("--build-context=dir2=%s", data.Get("dir2")))
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}

// TestBuildSourceDateEpoch tests that $SOURCE_DATE_EPOCH is propagated from the client env
// https://github.com/docker/buildx/pull/1482
func TestBuildSourceDateEpoch(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			test.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
ARG SOURCE_DATE_EPOCH
RUN echo $SOURCE_DATE_EPOCH >/source-date-epoch
CMD ["cat", "/source-date-epoch"]
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
		},
		SubTests: []*test.Case{
			{
				Description: "1111111111",
				Env: map[string]string{
					"SOURCE_DATE_EPOCH": "1111111111",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "-t", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("1111111111\n")),
			},
			{
				Description: "2222222222",
				Env: map[string]string{
					"SOURCE_DATE_EPOCH": "1111111111",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Get("buildCtx"), "--build-arg", "SOURCE_DATE_EPOCH=2222222222", "-t", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Expected: test.Expects(0, nil, test.Equals("2222222222\n")),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildNetwork(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			test.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
RUN apk add --no-cache curl
RUN curl -I http://google.com
	`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
		},
		SubTests: []*test.Case{
			{
				Description: "none",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", data.Get("buildCtx"), "-t", data.Identifier(), "--no-cache", "--network", "none")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(1, nil, nil),
			},
			{
				Description: "empty",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", data.Get("buildCtx"), "-t", data.Identifier(), "--no-cache", "--network", "")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "default",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", data.Get("buildCtx"), "-t", data.Identifier(), "--no-cache", "--network", "default")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildAttestation(t *testing.T) {
	nerdtest.Setup()

	const testSBOMFileName = "sbom.spdx.json"
	const testProvenanceFileName = "provenance.json"

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			test.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if nerdtest.IsDocker() {
				helpers.Anyhow("buildx", "rm", data.Identifier("builder"))
			}
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			if nerdtest.IsDocker() {
				helpers.Anyhow("buildx", "create", "--name", data.Identifier("builder"), "--bootstrap", "--use")
			}

			dockerfile := fmt.Sprintf(`FROM %s`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
		},
		SubTests: []*test.Case{
			{
				Description: "SBOM",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					outputSBOMDir := helpers.T().TempDir()
					data.Set("outputSBOMFile", filepath.Join(outputSBOMDir, testSBOMFileName))

					cmd := helpers.Command("build")
					if nerdtest.IsDocker() {
						cmd.WithArgs("--builder", data.Identifier("builder"))
					}
					cmd.WithArgs("--sbom=true", "-o", fmt.Sprintf("type=local,dest=%s", outputSBOMDir), data.Get("buildCtx"))
					return cmd
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							_, err := os.Stat(data.Get("outputSBOMFile"))
							assert.NilError(t, err, info)
						},
					}
				},
			},
			{
				Description: "Provenance",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					outputProvenanceDir := data.TempDir()
					data.Set("outputProvenanceFile", filepath.Join(outputProvenanceDir, testProvenanceFileName))

					cmd := helpers.Command("build")
					if nerdtest.IsDocker() {
						cmd.WithArgs("--builder", data.Identifier("builder"))
					}
					cmd.WithArgs("--provenance=mode=min", "-o", fmt.Sprintf("type=local,dest=%s", outputProvenanceDir), data.Get("buildCtx"))
					return cmd
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							_, err := os.Stat(data.Get("outputProvenanceFile"))
							assert.NilError(t, err, info)
						},
					}
				},
			},
			{
				Description: "Attestation",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					outputAttestationDir := data.TempDir()
					data.Set("outputSBOMFile", filepath.Join(outputAttestationDir, testSBOMFileName))
					data.Set("outputProvenanceFile", filepath.Join(outputAttestationDir, testProvenanceFileName))

					cmd := helpers.Command("build")
					if nerdtest.IsDocker() {
						cmd.WithArgs("--builder", data.Identifier("builder"))
					}
					cmd.WithArgs("--attest=type=provenance,mode=min", "--attest=type=sbom", "-o", fmt.Sprintf("type=local,dest=%s", outputAttestationDir), data.Get("buildCtx"))
					return cmd
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							_, err := os.Stat(data.Get("outputSBOMFile"))
							assert.NilError(t, err, info)
							_, err = os.Stat(data.Get("outputProvenanceFile"))
							assert.NilError(t, err, info)
						},
					}
				},
			},
		},
	}

	testCase.Run(t)
}

func TestBuildAddHost(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
RUN ping -c 5 alpha
RUN ping -c 5 beta
`, testutil.CommonImage)
			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)
			data.Set("buildCtx", buildCtx)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("build", data.Get("buildCtx"), "-t", data.Identifier(), "--add-host", "alpha:127.0.0.1", "--add-host", "beta:127.0.0.1")
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}
