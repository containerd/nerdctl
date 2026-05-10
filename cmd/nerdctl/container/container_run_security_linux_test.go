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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/apparmorutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

const (
	CapNetRaw  = 13
	CapIPCLock = 14

	capEffShellCmd = "grep -w ^CapEff: /proc/self/status | sed -e \"s/^CapEff:[[:space:]]*//g\""
)

func getCapEff(helpers test.Helpers, args ...string) uint64 {
	fullArgs := []string{"run", "--rm"}
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs,
		testutil.AlpineImage,
		"sh",
		"-euc",
		capEffShellCmd,
	)
	s := strings.TrimSpace(helpers.Capture(fullArgs...))
	ui64, err := strconv.ParseUint(s, 16, 64)
	assert.NilError(helpers.T(), err)
	return ui64
}

func TestRunCap(t *testing.T) {
	testCase := nerdtest.Setup()

	// https://github.com/containerd/containerd/blob/9a9bd097564b0973bfdb0b39bf8262aa1b7da6aa/oci/spec.go#L93
	var defaultCaps uint64 = 0xa80425fb

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// allCaps varies depending on the target version and the kernel version.
		allCaps := getCapEff(helpers, "--privileged")
		helpers.T().Log(fmt.Sprintf("allCaps=%016x", allCaps))
		data.Labels().Set("allCaps", strconv.FormatUint(allCaps, 10))
	}

	capCmd := func(args ...string) func(test.Data, test.Helpers) test.TestableCommand {
		return func(data test.Data, helpers test.Helpers) test.TestableCommand {
			cmdArgs := append([]string{"run", "--rm"}, args...)
			cmdArgs = append(cmdArgs, testutil.AlpineImage, "sh", "-euc", capEffShellCmd)
			return helpers.Command(cmdArgs...)
		}
	}

	capExpected := func(capEffFn func(allCaps uint64) uint64) func(test.Data, test.Helpers) *test.Expected {
		return func(data test.Data, helpers test.Helpers) *test.Expected {
			allCaps, _ := strconv.ParseUint(data.Labels().Get("allCaps"), 10, 64)
			return &test.Expected{
				ExitCode: expect.ExitCodeSuccess,
				Output:   expect.Equals(fmt.Sprintf("%016x\n", capEffFn(allCaps))),
			}
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "default",
			Command:     capCmd(),
			Expected:    capExpected(func(allCaps uint64) uint64 { return allCaps & defaultCaps }),
		},
		{
			Description: "--cap-add=all",
			Command:     capCmd("--cap-add=all"),
			Expected:    capExpected(func(allCaps uint64) uint64 { return allCaps }),
		},
		{
			Description: "--cap-add=ipc_lock",
			Command:     capCmd("--cap-add=ipc_lock"),
			Expected:    capExpected(func(allCaps uint64) uint64 { return (allCaps & defaultCaps) | (1 << CapIPCLock) }),
		},
		{
			Description: "--cap-add=all --cap-drop=net_raw",
			Command:     capCmd("--cap-add=all", "--cap-drop=net_raw"),
			Expected:    capExpected(func(allCaps uint64) uint64 { return allCaps ^ (1 << CapNetRaw) }),
		},
		{
			Description: "--cap-drop=all --cap-add=net_raw",
			Command:     capCmd("--cap-drop=all", "--cap-add=net_raw"),
			Expected:    capExpected(func(allCaps uint64) uint64 { return 1 << CapNetRaw }),
		},
		{
			Description: "--cap-drop=all --cap-add=NET_RAW",
			Command:     capCmd("--cap-drop=all", "--cap-add=NET_RAW"),
			Expected:    capExpected(func(allCaps uint64) uint64 { return 1 << CapNetRaw }),
		},
		{
			Description: "--cap-drop=all --cap-add=cap_net_raw",
			Command:     capCmd("--cap-drop=all", "--cap-add=cap_net_raw"),
			Expected:    capExpected(func(allCaps uint64) uint64 { return 1 << CapNetRaw }),
		},
		{
			Description: "--cap-drop=all --cap-add=CAP_NET_RAW",
			Command:     capCmd("--cap-drop=all", "--cap-add=CAP_NET_RAW"),
			Expected:    capExpected(func(allCaps uint64) uint64 { return 1 << CapNetRaw }),
		},
	}

	testCase.Run(t)
}

func TestRunSecurityOptSeccomp(t *testing.T) {
	testCase := nerdtest.Setup()

	seccompCmd := func(args ...string) func(test.Data, test.Helpers) test.TestableCommand {
		return func(data test.Data, helpers test.Helpers) test.TestableCommand {
			cmdArgs := append([]string{"run", "--rm"}, args...)
			// NOTE: busybox grep does not support -oP \K
			cmdArgs = append(cmdArgs, testutil.AlpineImage, "grep", "-Eo", `^Seccomp:\s*([0-9]+)`, "/proc/1/status")
			return helpers.Command(cmdArgs...)
		}
	}

	seccompExpected := func(expectedSeccomp int) test.Manager {
		return test.Expects(0, nil, expect.Match(
			regexp.MustCompile(fmt.Sprintf(`Seccomp:\s*%d`, expectedSeccomp)),
		))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "default",
			Command:     seccompCmd(),
			Expected:    seccompExpected(2),
		},
		{
			Description: "seccomp=unconfined",
			Command:     seccompCmd("--security-opt", "seccomp=unconfined"),
			Expected:    seccompExpected(0),
		},
		{
			Description: "--privileged",
			Command:     seccompCmd("--privileged"),
			Expected:    seccompExpected(0),
		},
	}

	testCase.Run(t)
}

func TestRunApparmor(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		defaultProfile := fmt.Sprintf("%s-default", testutil.GetTarget())
		if !apparmorutil.CanLoadNewProfile() && !apparmorutil.CanApplySpecificExistingProfile(defaultProfile) {
			helpers.T().Skip(fmt.Sprintf("needs to be able to apply %q profile", defaultProfile))
		}
		data.Labels().Set("defaultProfile", defaultProfile)

		attrCurrentPath := "/proc/self/attr/apparmor/current"
		if _, err := os.Stat(attrCurrentPath); err != nil {
			attrCurrentPath = "/proc/self/attr/current"
		}
		data.Labels().Set("attrCurrentPath", attrCurrentPath)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "default profile is enforced",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", testutil.AlpineImage, "cat", data.Labels().Get("attrCurrentPath"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Equals(fmt.Sprintf("%s (enforce)\n", data.Labels().Get("defaultProfile"))),
				}
			},
		},
		{
			Description: "explicit default profile is enforced",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--security-opt", "apparmor="+data.Labels().Get("defaultProfile"),
					testutil.AlpineImage, "cat", data.Labels().Get("attrCurrentPath"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Equals(fmt.Sprintf("%s (enforce)\n", data.Labels().Get("defaultProfile"))),
				}
			},
		},
		{
			Description: "apparmor=unconfined",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--security-opt", "apparmor=unconfined",
					testutil.AlpineImage, "cat", data.Labels().Get("attrCurrentPath"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("unconfined")),
		},
		{
			Description: "privileged implies unconfined",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--privileged",
					testutil.AlpineImage, "cat", data.Labels().Get("attrCurrentPath"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("unconfined")),
		},
	}

	testCase.Run(t)
}

func TestRunSelinuxWithSecurityOpt(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Selinux
	testContainer := testutil.Identifier(t)

	testCase.SubTests = []*test.Case{
		{
			Description: "test run with selinux-enabled",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("--selinux-enabled", "run", "-d", "--security-opt", "label=type:container_t", "--name", testContainer, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", testContainer)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("container", "inspect", "--format", "{{.State.Pid}}", testContainer)
							pid := strings.TrimSpace(inspectOut)
							fileName := fmt.Sprintf("/proc/%s/attr/current", pid)
							data, err := os.ReadFile(fileName)
							assert.NilError(t, err)
							assert.Equal(t, strings.Contains(string(data), "container_t"), true)
						},
					),
				}
			},
		},
	}
	testCase.Run(t)
}
func TestRunSelinux(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Selinux
	testContainer := testutil.Identifier(t)

	testCase.SubTests = []*test.Case{
		{
			Description: "test run with selinux-enabled",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("--selinux-enabled", "run", "-d", "--name", testContainer, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", testContainer)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("container", "inspect", "--format", "{{.State.Pid}}", testContainer)
							pid := strings.TrimSpace(inspectOut)
							fileName := fmt.Sprintf("/proc/%s/attr/current", pid)
							data, err := os.ReadFile(fileName)
							assert.NilError(t, err)
							assert.Equal(t, strings.Contains(string(data), "container_t"), true)
						},
					),
				}
			},
		},
	}
	testCase.Run(t)
}

func TestRunSelinuxWithVolumeLabel(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Selinux
	testContainer := testutil.Identifier(t)

	testCase.SubTests = []*test.Case{
		{
			Description: "test run with selinux-enabled",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("--selinux-enabled", "run", "-d", "-v", fmt.Sprintf("/%s:/%s:Z", testContainer, testContainer), "--name", testContainer, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", testContainer)
				os.RemoveAll(fmt.Sprintf("/%s", testContainer))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						func(stdout string, t tig.T) {
							cmd := exec.Command("ls", "-Z", fmt.Sprintf("/%s", testContainer))
							lsStdout, err := cmd.CombinedOutput()
							assert.NilError(t, err)
							assert.Equal(t, strings.Contains(string(lsStdout), "container_t"), true)
						},
					),
				}
			},
		},
	}
	testCase.Run(t)
}

// TestRunSeccompCapSysPtrace tests https://github.com/containerd/nerdctl/issues/976
func TestRunSeccompCapSysPtrace(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--cap-add", "sys_ptrace", testutil.AlpineImage, "sh", "-euxc", "apk add -q strace && strace true")
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
	// Docker/Moby 's seccomp profile allows ptrace(2) by default, but containerd does not (yet): https://github.com/containerd/containerd/issues/6802
}

func TestRunSystemPathsUnconfined(t *testing.T) {
	testCase := nerdtest.Setup()

	const findmntRCmd = "apk add -q findmnt && findmnt -R /proc && findmnt -R /sys"

	testCase.SubTests = []*test.Case{
		{
			Description: "masked paths are unconfined",
			Setup: func(data test.Data, helpers test.Helpers) {
				defaultOut := helpers.Capture("run", "--rm", testutil.AlpineImage, "sh", "-euc", findmntRCmd)

				var confined []string
				for _, path := range []string{
					"/proc/kcore",
					"/proc/keys",
					"/proc/latency_stats",
					"/proc/sched_debug",
					"/proc/scsi",
					"/proc/timer_list",
					"/proc/timer_stats",
					"/sys/firmware",
					"/sys/fs/selinux",
				} {
					// Not each distribution will support every masked path here.
					if strings.Contains(defaultOut, path) {
						confined = append(confined, path)
					}
				}

				assert.Check(helpers.T(), len(confined) != 0, "Default container has no confined paths to validate")
				data.Labels().Set("confined", strings.Join(confined, ","))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--security-opt", "systempaths=unconfined",
					testutil.AlpineImage, "sh", "-euc", findmntRCmd)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				confined := strings.Split(data.Labels().Get("confined"), ",")
				comparators := make([]test.Comparator, 0, len(confined))
				for _, path := range confined {
					comparators = append(comparators, expect.DoesNotContain(path))
				}
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.All(comparators...),
				}
			},
		},
	}

	for _, path := range []string{
		"/proc/acpi",
		"/proc/bus",
		"/proc/fs",
		"/proc/irq",
		"/proc/sysrq-trigger",
		"/proc/sys",
	} {
		// Not each distribution will support every read-only path here.
		findmntCmd := fmt.Sprintf("apk add -q findmnt && findmnt %s || true", path)
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: "path " + path + " is writable when unconfined",
			Setup: func(data test.Data, helpers test.Helpers) {
				out := helpers.Capture("run", "--rm", testutil.AlpineImage, "sh", "-euc", findmntCmd)
				if !strings.Contains(out, path) {
					helpers.T().Skip(fmt.Sprintf("%s not present, skipping", path))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--security-opt", "systempaths=unconfined",
					testutil.AlpineImage, "sh", "-euc", findmntCmd)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.DoesNotContain("ro,")),
		})
	}

	testCase.Run(t)
}

func TestRunPrivileged(t *testing.T) {
	testCase := nerdtest.Setup()

	// docker does not support --privileged-without-host-devices
	testCase.Require = require.All(require.Not(nerdtest.Docker), require.Not(nerdtest.Rootless))
	testCase.NoParallel = true

	const devPath = "/dev/dummy-zero"

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// a dummy zero device: mknod /dev/dummy-zero c 1 5
		helpers.Custom("mknod", devPath, "c", "1", "5").Run(&test.Expected{ExitCode: expect.ExitCodeSuccess})
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Custom("rm", "-f", devPath).Run(nil)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with host devices",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--privileged", testutil.AlpineImage, "ls", devPath)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals(devPath+"\n")),
		},
		{
			Description: "without host devices",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--privileged",
					"--security-opt", "privileged-without-host-devices", testutil.AlpineImage, "ls", devPath)
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, []error{errors.New("No such file or directory")}, nil),
		},
	}

	testCase.Run(t)
}
