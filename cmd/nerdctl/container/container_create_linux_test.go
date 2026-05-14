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
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/opencontainers/go-digest"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

func TestCreateWithLabel(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("create", "--name", data.Identifier(), "--label", "foo=bar", testutil.NginxAlpineImage, "echo", "foo")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				fooLabel := strings.TrimSpace(helpers.Capture("inspect", "--format", `{{index .Config.Labels "foo"}}`, data.Identifier()))
				assert.Equal(t, "bar", fooLabel)
				maintainerLabel := strings.TrimSpace(helpers.Capture("inspect", "--format", `{{index .Config.Labels "maintainer"}}`, data.Identifier()))
				assert.Equal(t, "NGINX Docker Maintainers <docker-maint@nginx.com>", maintainerLabel)
			},
		}
	}
	testCase.Run(t)
}

func TestCreateWithMACAddress(t *testing.T) {
	testCase := nerdtest.Setup()

	const (
		networkBridgeKey  = "networkBridge"
		networkMACvlanKey = "networkMACvlan"
		networkIPvlanKey  = "networkIPvlan"
		defaultMacKey     = "defaultMac"
		macAddressKey     = "macAddress"
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set(networkBridgeKey, "testNetworkBridge"+data.Identifier())
		data.Labels().Set(networkMACvlanKey, "testNetworkMACvlan"+data.Identifier())
		data.Labels().Set(networkIPvlanKey, "testNetworkIPvlan"+data.Identifier())
		helpers.Ensure("network", "create", data.Labels().Get(networkBridgeKey), "--driver", "bridge")
		helpers.Ensure("network", "create", data.Labels().Get(networkMACvlanKey), "--driver", "macvlan")
		helpers.Ensure("network", "create", data.Labels().Get(networkIPvlanKey), "--driver", "ipvlan")
		defaultMac := strings.TrimSpace(helpers.Capture("run", "--rm", "--network", "host",
			testutil.CommonImage, "sh", "-c", "ip addr show eth0 | grep ether | awk '{printf $2}'"))
		data.Labels().Set(defaultMacKey, defaultMac)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("network", "rm", data.Labels().Get(networkBridgeKey))
		helpers.Anyhow("network", "rm", data.Labels().Get(networkMACvlanKey))
		helpers.Anyhow("network", "rm", data.Labels().Get(networkIPvlanKey))
	}

	setupMAC := func(data test.Data, helpers test.Helpers) {
		macAddress, err := nettestutil.GenerateMACAddress()
		assert.NilError(helpers.T(), err, "failed to generate MAC address")
		data.Labels().Set(macAddressKey, macAddress)
	}

	makeCreateCommand := func(network string) test.Executor {
		return func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("create",
				"--network", network,
				"--name", data.Identifier(),
				"--mac-address", data.Labels().Get(macAddressKey),
				testutil.CommonImage, "sh", "-c", "--", "ip addr show")
		}
	}
	makeDynamicCreateCommand := func(networkKey string) test.Executor {
		return func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("create",
				"--network", data.Labels().Get(networkKey),
				"--name", data.Identifier(),
				"--mac-address", data.Labels().Get(macAddressKey),
				testutil.CommonImage, "sh", "-c", "--", "ip addr show")
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "host network - container inherits host MAC",
			Setup:       setupMAC,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: makeCreateCommand("host"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						startOut := helpers.Capture("start", "-a", data.Identifier())
						assert.Assert(t, strings.Contains(startOut, data.Labels().Get(defaultMacKey)),
							fmt.Sprintf("expected start output to contain %q: %q", data.Labels().Get(defaultMacKey), startOut))
					},
				}
			},
		},
		{
			Description: "none network - MAC address flag is accepted",
			Setup:       setupMAC,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: makeCreateCommand("none"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						helpers.Ensure("start", "-a", data.Identifier())
					},
				}
			},
		},
		{
			Description: "container network - nonexistent container fails",
			Setup:       setupMAC,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: makeCreateCommand("container:nonexistent-container-for-test"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				if nerdtest.IsDocker() {
					// Docker delays the failure to start time
					return &test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t tig.T) {
							helpers.Command("start", "-i", "-a", data.Identifier()).
								Run(&test.Expected{
									ExitCode: 1,
									Errors:   []error{errors.New("container")},
								})
						},
					}
				}
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("container")},
				}
			},
		},
		{
			Description: "bridge network - MAC address is applied",
			Setup:       setupMAC,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: makeCreateCommand("bridge"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						startOut := helpers.Capture("start", "-a", data.Identifier())
						assert.Assert(t, strings.Contains(startOut, data.Labels().Get(macAddressKey)),
							fmt.Sprintf("expected start output to contain %q: %q", data.Labels().Get(macAddressKey), startOut))
					},
				}
			},
		},
		{
			Description: "custom bridge network - MAC address is applied",
			Setup:       setupMAC,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: makeDynamicCreateCommand(networkBridgeKey),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						startOut := helpers.Capture("start", "-a", data.Identifier())
						assert.Assert(t, strings.Contains(startOut, data.Labels().Get(macAddressKey)),
							fmt.Sprintf("expected start output to contain %q: %q", data.Labels().Get(macAddressKey), startOut))
					},
				}
			},
		},
		{
			Description: "macvlan network - MAC address is applied",
			Setup:       setupMAC,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: makeDynamicCreateCommand(networkMACvlanKey),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						startOut := helpers.Capture("start", "-a", data.Identifier())
						assert.Assert(t, strings.Contains(startOut, data.Labels().Get(macAddressKey)),
							fmt.Sprintf("expected start output to contain %q: %q", data.Labels().Get(macAddressKey), startOut))
					},
				}
			},
		},
		{
			Description: "ipvlan network - MAC address setting not supported",
			Setup:       setupMAC,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: makeDynamicCreateCommand(networkIPvlanKey),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				if nerdtest.IsDocker() {
					// Docker delays the failure to start time
					return &test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t tig.T) {
							helpers.Command("start", "-i", "-a", data.Identifier()).
								Run(&test.Expected{
									ExitCode: 1,
									Errors:   []error{errors.New("not support")},
								})
						},
					}
				}
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("not support")},
				}
			},
		},
	}
	testCase.Run(t)
}

func TestCreateWithTty(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.SubTests = []*test.Case{
		{
			Description: "create without tty - stty exits with error",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("create", "--name", data.Identifier(), testutil.CommonImage, "stty")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("start", "-a", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("stty: standard input: Not a tty")},
				}
			},
		},
		{
			Description: "create with tty - stty succeeds",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("create", "-t", "--name", data.Identifier(), testutil.CommonImage, "stty")
				// start without -a: tty output is not forwarded over a pipe, so
				// capturing it via "start -a" is unreliable. Use "logs" instead,
				// which reads from the containerd log driver regardless of tty.
				helpers.Ensure("start", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Contains("speed 38400 baud; line = 0;"),
				}
			},
		},
	}
	testCase.Run(t)
}

// TestIssue2993 tests https://github.com/containerd/nerdctl/issues/2993
func TestIssue2993(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.Not(nerdtest.Docker)

	const (
		containersPathKey = "containersPath"
		etchostsPathKey   = "etchostsPath"
	)

	getAddrHash := func(addr string) string {
		const addrHashLen = 8

		d := digest.SHA256.FromString(addr)
		h := d.Encoded()[0:addrHashLen]

		return h
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Issue #2993 - nerdctl no longer leaks containers and etchosts directories and files when container creation fails.",
			Setup: func(data test.Data, helpers test.Helpers) {
				dataRoot := data.Temp().Path()

				helpers.Ensure("run", "--data-root", dataRoot, "--name", data.Identifier(), "-d", testutil.AlpineImage, "sleep", nerdtest.Infinity)

				h := getAddrHash(defaults.DefaultAddress)
				dataStore := filepath.Join(dataRoot, h)

				namespace := string(helpers.Read(nerdtest.Namespace))

				containersPath := filepath.Join(dataStore, "containers", namespace)
				containersDirs, err := os.ReadDir(containersPath)
				assert.NilError(t, err)
				assert.Equal(t, len(containersDirs), 1)

				etchostsPath := filepath.Join(dataStore, "etchosts", namespace)
				etchostsDirs, err := os.ReadDir(etchostsPath)
				assert.NilError(t, err)
				assert.Equal(t, len(etchostsDirs), 1)

				data.Labels().Set(containersPathKey, containersPath)
				data.Labels().Set(etchostsPathKey, etchostsPath)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "--data-root", data.Temp().Path(), "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--data-root", data.Temp().Path(), "--name", data.Identifier(), "-d", testutil.AlpineImage, "sleep", nerdtest.Infinity)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("is already used by ID")},
					Output: func(stdout string, t tig.T) {
						containersDirs, err := os.ReadDir(data.Labels().Get(containersPathKey))
						assert.NilError(t, err)
						assert.Equal(t, len(containersDirs), 1)

						etchostsDirs, err := os.ReadDir(data.Labels().Get(etchostsPathKey))
						assert.NilError(t, err)
						assert.Equal(t, len(etchostsDirs), 1)
					},
				}
			},
		},
		{
			Description: "Issue #2993 - nerdctl no longer leaks containers and etchosts directories and files when containers are removed.",
			Setup: func(data test.Data, helpers test.Helpers) {
				dataRoot := data.Temp().Path()

				helpers.Ensure("run", "--data-root", dataRoot, "--name", data.Identifier(), "-d", testutil.AlpineImage, "sleep", nerdtest.Infinity)

				h := getAddrHash(defaults.DefaultAddress)
				dataStore := filepath.Join(dataRoot, h)

				namespace := string(helpers.Read(nerdtest.Namespace))

				containersPath := filepath.Join(dataStore, "containers", namespace)
				containersDirs, err := os.ReadDir(containersPath)
				assert.NilError(t, err)
				assert.Equal(t, len(containersDirs), 1)

				etchostsPath := filepath.Join(dataStore, "etchosts", namespace)
				etchostsDirs, err := os.ReadDir(etchostsPath)
				assert.NilError(t, err)
				assert.Equal(t, len(etchostsDirs), 1)

				data.Labels().Set(containersPathKey, containersPath)
				data.Labels().Set(etchostsPathKey, etchostsPath)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("--data-root", data.Temp().Path(), "rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("--data-root", data.Temp().Path(), "rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, t tig.T) {
						containersDirs, err := os.ReadDir(data.Labels().Get(containersPathKey))
						assert.NilError(t, err)
						assert.Equal(t, len(containersDirs), 0)

						etchostsDirs, err := os.ReadDir(data.Labels().Get(etchostsPathKey))
						assert.NilError(t, err)
						assert.Equal(t, len(etchostsDirs), 0)
					},
				}
			},
		},
	}

	testCase.Run(t)
}

func TestCreateFromOCIArchive(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		nerdtest.Build,
		require.Not(nerdtest.Docker),
	)

	const sentinel = "test-nerdctl-create-from-oci-archive"

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf("FROM %s\nCMD [\"echo\", \"%s\"]", testutil.CommonImage, sentinel)
		err := os.WriteFile(filepath.Join(data.Temp().Path(), "Dockerfile"), []byte(dockerfile), 0644)
		assert.NilError(helpers.T(), err)

		imageName := data.Identifier("image") + ":latest"
		tarPath := data.Temp().Path("image.tar")
		data.Labels().Set("imageName", imageName)
		data.Labels().Set("tarPath", tarPath)

		helpers.Ensure("build", "--tag", imageName,
			fmt.Sprintf("--output=type=oci,dest=%s", tarPath),
			data.Temp().Path())
		helpers.Ensure("create", "--rm", "--name", data.Identifier(),
			fmt.Sprintf("oci-archive://%s", tarPath))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("rmi", "-f", data.Labels().Get("imageName"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("start", "--attach", data.Identifier())
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(stdout, sentinel),
					fmt.Sprintf("expected stdout to contain %q: %q", sentinel, stdout))
			},
		}
	}
	testCase.Run(t)
}

func TestUsernsMappingCreateCmd(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		nerdtest.AllowModifyUserns,
		nerdtest.RemapIDs,
		require.Not(nerdtest.Docker))
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("validUserns", "nerdctltestuser")
		data.Labels().Set("expectedHostUID", "123456789")
		data.Labels().Set("invalidUserns", "invaliduser")
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "Test container create with valid Userns",
			NoParallel:  true, // Changes system config so running in non parallel mode
			Setup: func(data test.Data, helpers test.Helpers) {
				err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
				assert.NilError(helpers.T(), err, "Failed to append Userns config")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				removeUsernsConfig(helpers.T(), data.Labels().Get("validUserns"), helpers)
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				helpers.Ensure("create", "--tty", "--userns-remap", data.Labels().Get("validUserns"), "--name", data.Identifier(), testutil.NginxAlpineImage)
				return helpers.Command("start", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
						assert.NilError(t, err, "Failed to get container host UID")
						assert.Assert(t, actualHostUID == data.Labels().Get("expectedHostUID"))
					},
				}
			},
		},
		{
			Description: "Test container create failure with valid Userns and privileged flag",
			NoParallel:  true, // Changes system config so running in non parallel mode
			Setup: func(data test.Data, helpers test.Helpers) {
				err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
				assert.NilError(helpers.T(), err, "Failed to append Userns config")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				removeUsernsConfig(helpers.T(), data.Labels().Get("validUserns"), helpers)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("create", "--tty", "--privileged", "--userns-remap", data.Labels().Get("validUserns"), "--name", data.Identifier(), testutil.NginxAlpineImage)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
				}
			},
		},
		{
			Description: "Test container create with invalid Userns",
			NoParallel:  true, // Changes system config so running in non parallel mode
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("create", "--tty", "--userns-remap", data.Labels().Get("invalidUserns"), "--name", data.Identifier(), testutil.NginxAlpineImage)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
				}
			},
		},
	}
	testCase.Run(t)
}

func getContainerHostUID(helpers test.Helpers, containerName string) (string, error) {
	result := helpers.Capture("inspect", "--format", "{{.State.Pid}}", containerName)
	pidStr := strings.TrimSpace(result)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return "", fmt.Errorf("invalid PID: %v", err)
	}

	stat, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return "", fmt.Errorf("failed to stat process: %v", err)
	}

	uid := int(stat.Sys().(*syscall.Stat_t).Uid)
	return strconv.Itoa(uid), nil
}

func appendUsernsConfig(userns string, hostUID string, helpers test.Helpers) error {
	addUser(userns, hostUID, helpers)
	entry := fmt.Sprintf("%s:%s:65536\n", userns, hostUID)
	tempDir := helpers.T().TempDir()
	files := []string{"subuid", "subgid"}
	for _, file := range files {

		fileBak := filepath.Join(tempDir, file)
		defer os.Remove(fileBak)
		d, err := os.Create(fileBak)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", fileBak, err)
		}

		s, err := os.Open(filepath.Join("/etc", file))
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", file, err)
		}
		defer s.Close()

		_, err = io.Copy(d, s)
		if err != nil {
			return fmt.Errorf("failed to copy %s to %s: %w", file, fileBak, err)
		}

		f, err := os.OpenFile(fmt.Sprintf("/etc/%s", file), os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", file, err)
		}
		defer f.Close()

		if _, err := f.WriteString(entry); err != nil {
			return fmt.Errorf("failed to write to %s: %w", file, err)
		}
	}
	return nil
}

func addUser(username string, hostID string, helpers test.Helpers) {
	helpers.Custom("groupadd", "-g", hostID, username).Run(&test.Expected{
		ExitCode: 0})
	helpers.Custom("useradd", "-u", hostID, "-g", hostID, "-s", "/bin/false", username).Run(&test.Expected{
		ExitCode: 0})
}

func removeUsernsConfig(t tig.T, userns string, helpers test.Helpers) {
	delUser(userns, helpers)
	delGroup(userns, helpers)
	tempDir := t.TempDir()
	files := []string{"subuid", "subgid"}
	for _, file := range files {
		fileBak := filepath.Join(tempDir, file)
		s, err := os.Open(fileBak)
		if err != nil {
			t.Log(fmt.Sprintf("failed to open %s, Error: %s", fileBak, err))
			continue
		}
		defer s.Close()

		d, err := os.Open(filepath.Join("/etc/%s", file))
		if err != nil {
			t.Log(fmt.Sprintf("failed to open %s, Error: %s", file, err))
			continue
		}
		defer d.Close()

		_, err = io.Copy(d, s)
		if err != nil {
			t.Log(fmt.Sprintf("failed to restore. Copy %s to %s failed, Error %s", fileBak, file, err))
			continue
		}
	}
}

func delUser(username string, helpers test.Helpers) {
	helpers.Custom("userdel", username).Run(&test.Expected{ExitCode: expect.ExitCodeNoCheck})
}

func delGroup(groupname string, helpers test.Helpers) {
	helpers.Custom("groupdel", groupname).Run(&test.Expected{ExitCode: expect.ExitCodeNoCheck})
}
