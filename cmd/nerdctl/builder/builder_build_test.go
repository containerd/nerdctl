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
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestBuildBasics(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path())
		},
		SubTests: []*test.Case{
			{
				Description: "Successfully build with 'tag first', 'buildctx second'",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", "-t", data.Identifier(), data.Labels().Get("buildCtx"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "Successfully build with 'buildctx first', 'tag second'",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "Successfully build with output docker, main tag still works",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure(
						"build",
						data.Labels().Get("buildCtx"),
						"-t",
						data.Identifier(),
						"--output=type=docker,name="+data.Identifier("ignored"),
					)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "Successfully build with output docker, name cannot be used",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure(
						"build",
						data.Labels().Get("buildCtx"),
						"-t",
						data.Identifier(),
						"--output=type=docker,name="+data.Identifier("ignored"),
					)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier("ignored"))
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier("ignored"))
				},
				Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
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

			data.Labels().Set("OS", "linux")
			data.Labels().Set("Architecture", candidateArch)
			return can, "Current environment does not support emulation"
		},
	}

	dockerfile := fmt.Sprintf(`FROM %s
RUN echo hello > /hello
CMD ["echo", "nerdctl-build-test-string"]`, testutil.CommonImage)

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			requireEmulation,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command(
				"build",
				data.Labels().Get("buildCtx"),
				"--platform",
				fmt.Sprintf("%s/%s", data.Labels().Get("OS"), data.Labels().Get("Architecture")),
				"-t",
				data.Identifier(),
			)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
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
			data.Temp().Save(dockerfile, "Dockerfile")
			helpers.Ensure("build", "-t", data.Identifier("first"), data.Temp().Path())

			dockerfileSecond := fmt.Sprintf(`FROM %s
RUN echo hello2 > /hello2
CMD ["cat", "/hello2"]`, data.Identifier("first"))
			data.Temp().Save(dockerfileSecond, "Dockerfile")
			helpers.Ensure("build", "-t", data.Identifier("second"), data.Temp().Path())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier("second"))
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hello2\n")),
	}

	testCase.Run(t)
}

// TestBuildFromContainerd tests if an image can be built on an image pulled by nerdctl.
// This isn't currently supported by nerdctl with BuildKit OCI worker.
func TestBuildFromContainerd(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			require.Not(nerdtest.Docker),
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
			data.Temp().Save(dockerfile, "Dockerfile")
			helpers.Ensure("build", "-t", data.Identifier("second"), data.Temp().Path())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier("second"))
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hello2\n")),
	}

	testCase.Run(t)
}

func TestBuildFromStdin(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-stdin"]`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "-", ".")
			cmd.Feed(strings.NewReader(dockerfile))
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

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-dockerfile"]
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "test", "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path("test"))
		},
		SubTests: []*test.Case{
			{
				Description: "Dockerfile ..",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "Dockerfile", "..")
					cmd.WithCwd(data.Labels().Get("buildCtx"))
					return cmd
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
			},
			{
				Description: "Dockerfile .",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "Dockerfile", ".")
					cmd.WithCwd(data.Labels().Get("buildCtx"))
					return cmd
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
			},
			{
				Description: "../Dockerfile .",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build", "-t", data.Identifier(), "-f", "../Dockerfile", ".")
					cmd.WithCwd(data.Labels().Get("buildCtx"))
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

	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /`, testFileName)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Temp().Save(testContent, testFileName)
			data.Labels().Set("buildCtx", data.Temp().Path())
		},
		SubTests: []*test.Case{
			{
				// GOTCHA: avoid comma and = in the test name, or buildctl will misparse the destination direction
				Description: "-o type local destination DIR: verify the file copied from context is in the output directory",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", "-o", fmt.Sprintf("type=local,dest=%s", data.Temp().Path()), data.Labels().Get("buildCtx"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout, info string, t *testing.T) {
							// Expecting testFileName to exist inside the output target directory
							assert.Equal(t, data.Temp().Load(testFileName), testContent, "file content is identical")
						},
					}
				},
			},
			{
				Description: "-o DIR: verify the file copied from context is in the output directory",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", "-o", data.Temp().Path(), data.Labels().Get("buildCtx"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout, info string, t *testing.T) {
							assert.Equal(t, data.Temp().Load(testFileName), testContent, "file content is identical")
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

	dockerfile := fmt.Sprintf(`FROM %s
ARG TEST_STRING=1
ENV TEST_STRING=$TEST_STRING
CMD echo $TEST_STRING
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path())
		},
		SubTests: []*test.Case{
			{
				Description: "No args",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("1\n")),
			},
			{
				Description: "ArgValueOverridesDefault",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "--build-arg", "TEST_STRING=2", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("2\n")),
			},
			{
				Description: "EmptyArgValueOverridesDefault",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "--build-arg", "TEST_STRING=", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("\n")),
			},
			{
				Description: "UnsetArgKeyPreservesDefault",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "--build-arg", "TEST_STRING", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("1\n")),
			},
			{
				Description: "EnvValueOverridesDefault",
				Env: map[string]string{
					"TEST_STRING": "3",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "--build-arg", "TEST_STRING", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("3\n")),
			},
			{
				Description: "EmptyEnvValueOverridesDefault",
				Env: map[string]string{
					"TEST_STRING": "",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "--build-arg", "TEST_STRING", "-t", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("\n")),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildWithIIDFile(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			helpers.Ensure("build", data.Temp().Path(), "--iidfile", data.Temp().Path("id.txt"), "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Temp().Load("id.txt"))
		},

		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
	}

	testCase.Run(t)
}

func TestBuildWithLabels(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
LABEL name=nerdctl-build-test-label
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			helpers.Ensure("build", data.Temp().Path(), "--label", "label=test", "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("inspect", data.Identifier(), "--format", "{{json .Config.Labels }}")
		},

		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("{\"label\":\"test\",\"name\":\"nerdctl-build-test-label\"}\n")),
	}

	testCase.Run(t)
}

func TestBuildMultipleTags(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Labels().Get("i1"))
			helpers.Anyhow("rmi", "-f", data.Labels().Get("i2"))
			helpers.Anyhow("rmi", "-f", data.Labels().Get("i3"))
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Labels().Set("i1", data.Identifier("image"))
			data.Labels().Set("i2", data.Identifier("image2"))
			data.Labels().Set("i3", data.Identifier("image3")+":hello")
			data.Temp().Save(dockerfile, "Dockerfile")
			helpers.Ensure(
				"build",
				data.Temp().Path(),
				"-t", data.Labels().Get("i1"),
				"-t", data.Labels().Get("i2"),
				"-t", data.Labels().Get("i3"),
			)
		},
		SubTests: []*test.Case{
			{
				Description: "i1",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Labels().Get("i1"))
				},

				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "i2",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Labels().Get("i2"))
				},

				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "i3",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Labels().Get("i3"))
				},

				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildWithContainerfile(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			require.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			helpers.Ensure("build", data.Temp().Path(), "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier())
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nerdctl-build-test-string\n")),
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
			data.Temp().Save(dockerfile, "Dockerfile")

			dockerfile = fmt.Sprintf(`FROM %s
CMD ["echo", "containerfile"]
	`, testutil.CommonImage)
			data.Temp().Save(dockerfile, "Containerfile")

			helpers.Ensure("build", data.Temp().Path(), "-t", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", data.Identifier())
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("dockerfile\n")),
	}

	testCase.Run(t)
}

func TestBuildNoTag(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	// FIXME: this test should be rewritten and instead get the image id from the build, then query the image explicitly - instead of pruning / noparallel
	testCase := &test.Case{
		NoParallel: true,
		Require:    nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("image", "prune", "--force", "--all")
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")

			// XXX FIXME
			helpers.Capture("build", data.Temp().Path())
		},
		Command:  test.Command("images"),
		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("<none>")),
	}

	testCase.Run(t)
}

func TestBuildContextDockerImageAlias(t *testing.T) {
	nerdtest.Setup()

	dockerfile := `FROM myorg/myapp
CMD ["echo", "nerdctl-build-myorg/myapp"]`

	testCase := &test.Case{
		Require: nerdtest.Build,
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command(
				"build",
				"-t",
				data.Identifier(),
				data.Temp().Path(),
				fmt.Sprintf("--build-context=myorg/myapp=docker-image://%s", testutil.CommonImage),
			)
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
	}

	testCase.Run(t)
}

func TestBuildContextWithCopyFromDir(t *testing.T) {
	nerdtest.Setup()

	content := "hello_from_dir_2"
	filename := "hello.txt"
	dockerfile := fmt.Sprintf(`FROM %s
COPY --from=dir2 /%s /hello_from_dir2.txt
RUN ["cat", "/hello_from_dir2.txt"]`, testutil.CommonImage, filename)

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			require.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "context", "Dockerfile")
			data.Temp().Save(content, "other-directory", filename)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command(
				"build",
				"-t",
				data.Identifier(),
				data.Temp().Path("context"),
				fmt.Sprintf("--build-context=dir2=%s", data.Temp().Path("other-directory")),
			)
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
	}

	testCase.Run(t)
}

// TestBuildSourceDateEpoch tests that $SOURCE_DATE_EPOCH is propagated from the client env
// https://github.com/docker/buildx/pull/1482
func TestBuildSourceDateEpoch(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
ARG SOURCE_DATE_EPOCH
RUN echo $SOURCE_DATE_EPOCH >/source-date-epoch
CMD ["cat", "/source-date-epoch"]
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			require.Not(nerdtest.Docker),
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path())
		},
		SubTests: []*test.Case{
			{
				Description: "1111111111",
				Env: map[string]string{
					"SOURCE_DATE_EPOCH": "1111111111",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "-t", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("1111111111\n")),
			},
			{
				Description: "2222222222",
				Env: map[string]string{
					"SOURCE_DATE_EPOCH": "1111111111",
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("build", data.Labels().Get("buildCtx"), "--build-arg", "SOURCE_DATE_EPOCH=2222222222", "-t", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("2222222222\n")),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildNetwork(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
RUN apk add --no-cache curl
RUN curl -I http://google.com
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			require.Not(nerdtest.Docker),
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path())
		},
		SubTests: []*test.Case{
			{
				Description: "none",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", data.Labels().Get("buildCtx"), "-t", data.Identifier(), "--no-cache", "--network", "none")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(1, nil, nil),
			},
			{
				Description: "empty",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", data.Labels().Get("buildCtx"), "-t", data.Identifier(), "--no-cache", "--network", "")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
			},
			{
				Description: "default",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("build", data.Labels().Get("buildCtx"), "-t", data.Identifier(), "--no-cache", "--network", "default")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestBuildAttestation(t *testing.T) {
	nerdtest.Setup()

	const testSBOMFileName = "sbom.spdx.json"
	const testProvenanceFileName = "provenance.json"

	dockerfile := fmt.Sprintf(`FROM %s`, testutil.CommonImage)

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			require.Not(nerdtest.Docker),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if nerdtest.IsDocker() {
				helpers.Anyhow("buildx", "rm", data.Identifier("builder"))
			}
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			if nerdtest.IsDocker() {
				helpers.Anyhow("buildx", "create", "--name", data.Identifier("builder"), "--bootstrap", "--use")
			}

			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path())
		},
		SubTests: []*test.Case{
			{
				Description: "SBOM",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build")
					if nerdtest.IsDocker() {
						cmd.WithArgs("--builder", data.Identifier("builder"))
					}
					cmd.WithArgs(
						"--sbom=true",
						"-o", fmt.Sprintf("type=local,dest=%s", data.Temp().Path("dir-for-bom")),
						data.Labels().Get("buildCtx"),
					)
					return cmd
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout, info string, t *testing.T) {
							data.Temp().Exists("dir-for-bom", testSBOMFileName)
						},
					}
				},
			},
			{
				Description: "Provenance",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build")
					if nerdtest.IsDocker() {
						cmd.WithArgs("--builder", data.Identifier("builder"))
					}
					cmd.WithArgs(
						"--provenance=mode=min",
						"-o", fmt.Sprintf("type=local,dest=%s", data.Temp().Path("dir-for-prov")),
						data.Labels().Get("buildCtx"),
					)
					return cmd
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout, info string, t *testing.T) {
							data.Temp().Exists("dir-for-prov", testProvenanceFileName)
						},
					}
				},
			},
			{
				Description: "Attestation",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					cmd := helpers.Command("build")
					if nerdtest.IsDocker() {
						cmd.WithArgs("--builder", data.Identifier("builder"))
					}
					cmd.WithArgs(
						"--attest=type=provenance,mode=min",
						"--attest=type=sbom",
						"-o", fmt.Sprintf("type=local,dest=%s", data.Temp().Path("dir-for-attest")),
						data.Labels().Get("buildCtx"),
					)
					return cmd
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout, info string, t *testing.T) {
							data.Temp().Exists("dir-for-attest", testSBOMFileName)
							data.Temp().Exists("dir-for-attest", testProvenanceFileName)
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

	dockerfile := fmt.Sprintf(`FROM %s
RUN ping -c 5 alpha
RUN ping -c 5 beta
`, testutil.CommonImage)

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command(
				"build", data.Temp().Path(),
				"-t", data.Identifier(),
				"--add-host", "alpha:127.0.0.1",
				"--add-host", "beta:127.0.0.1",
			)
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
	}

	testCase.Run(t)
}

func TestBuildWithBuildkitConfig(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.All(
			nerdtest.Build,
			require.Not(nerdtest.Docker),
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]`, testutil.CommonImage)
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", data.Temp().Path())

		},
		SubTests: []*test.Case{
			{
				Description: "build with buildkit-host",
				Setup: func(data test.Data, helpers test.Helpers) {
					// Get BuildkitAddr
					buildkitAddr, err := buildkitutil.GetBuildkitHost(testutil.Namespace)
					assert.NilError(helpers.T(), err)
					buildkitAddr = strings.TrimPrefix(buildkitAddr, "unix://")

					// Symlink the buildkit Socket for testing
					symlinkedBuildkitAddr := filepath.Join(data.Temp().Path(), "buildkit.sock")

					// Do a negative test to check the setup
					helpers.Fail("build", "-t", data.Identifier(), "--buildkit-host", fmt.Sprintf("unix://%s", symlinkedBuildkitAddr), data.Labels().Get("buildCtx"))

					// Test build with the symlinked socket
					cmd := helpers.Custom("ln", "-s", buildkitAddr, symlinkedBuildkitAddr)
					cmd.Run(&test.Expected{
						ExitCode: 0,
					})
					helpers.Ensure("build", "-t", data.Identifier(), "--buildkit-host", fmt.Sprintf("unix://%s", symlinkedBuildkitAddr), data.Labels().Get("buildCtx"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
			{
				Description: "build with env specified",
				Setup: func(data test.Data, helpers test.Helpers) {
					// Get BuildkitAddr
					buildkitAddr, err := buildkitutil.GetBuildkitHost(testutil.Namespace)
					assert.NilError(helpers.T(), err)
					buildkitAddr = strings.TrimPrefix(buildkitAddr, "unix://")

					// Symlink the buildkit Socket for testing
					symlinkedBuildkitAddr := filepath.Join(data.Temp().Path(), "buildkit-env.sock")

					// Do a negative test to ensure setting up the env variable is effective
					cmd := helpers.Command("build", "-t", data.Identifier(), data.Labels().Get("buildCtx"))
					cmd.Setenv("BUILDKIT_HOST", fmt.Sprintf("unix://%s", symlinkedBuildkitAddr))
					cmd.Run(&test.Expected{ExitCode: expect.ExitCodeGenericFail})

					// Symlink the buildkit socket for testing
					cmd = helpers.Custom("ln", "-s", buildkitAddr, symlinkedBuildkitAddr)
					cmd.Run(&test.Expected{
						ExitCode: 0,
					})

					cmd = helpers.Command("build", "-t", data.Identifier(), data.Labels().Get("buildCtx"))
					cmd.Setenv("BUILDKIT_HOST", fmt.Sprintf("unix://%s", symlinkedBuildkitAddr))
					cmd.Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Identifier())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Expected: test.Expects(0, nil, expect.Equals("nerdctl-build-test-string\n")),
			},
		},
	}
	testCase.Run(t)
}
