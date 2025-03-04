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
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

func TestCreateWithLabel(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("create", "--name", tID, "--label", "foo=bar", testutil.NginxAlpineImage, "echo", "foo").AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()
	inspect := base.InspectContainer(tID)
	assert.Equal(base.T, "bar", inspect.Config.Labels["foo"])
	// the label `maintainer`` is defined by image
	assert.Equal(base.T, "NGINX Docker Maintainers <docker-maint@nginx.com>", inspect.Config.Labels["maintainer"])
}

func TestCreateWithMACAddress(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	networkBridge := "testNetworkBridge" + tID
	networkMACvlan := "testNetworkMACvlan" + tID
	networkIPvlan := "testNetworkIPvlan" + tID

	tearDown := func() {
		base.Cmd("network", "rm", networkBridge).Run()
		base.Cmd("network", "rm", networkMACvlan).Run()
		base.Cmd("network", "rm", networkIPvlan).Run()
	}

	tearDown()
	t.Cleanup(tearDown)

	base.Cmd("network", "create", networkBridge, "--driver", "bridge").AssertOK()
	base.Cmd("network", "create", networkMACvlan, "--driver", "macvlan").AssertOK()
	base.Cmd("network", "create", networkIPvlan, "--driver", "ipvlan").AssertOK()

	defaultMac := base.Cmd("run", "--rm", "-i", "--network", "host", testutil.CommonImage).
		CmdOption(testutil.WithStdin(strings.NewReader("ip addr show eth0 | grep ether | awk '{printf $2}'"))).
		Run().Stdout()

	passedMac := "we expect the generated mac on the output"
	tests := []struct {
		Network string
		WantErr bool
		Expect  string
	}{
		{"host", false, defaultMac}, // anything but the actual address being passed
		{"none", false, ""},
		{"container:whatever" + tID, true, "container"}, // "No such container" vs. "could not find container"
		{"bridge", false, passedMac},
		{networkBridge, false, passedMac},
		{networkMACvlan, false, passedMac},
		{networkIPvlan, true, "not support"},
	}
	for i, test := range tests {
		containerName := fmt.Sprintf("%s_%d", tID, i)
		testName := fmt.Sprintf("%s_container:%s_network:%s_expect:%s", tID, containerName, test.Network, test.Expect)
		expect := test.Expect
		network := test.Network
		wantErr := test.WantErr
		t.Run(testName, func(tt *testing.T) {
			tt.Parallel()

			macAddress, err := nettestutil.GenerateMACAddress()
			if err != nil {
				tt.Errorf("failed to generate MAC address: %s", err)
			}
			if expect == passedMac {
				expect = macAddress
			}
			tearDown := func() {
				base.Cmd("rm", "-f", containerName).Run()
			}
			tearDown()
			tt.Cleanup(tearDown)
			// This is currently blocked by https://github.com/containerd/nerdctl/pull/3104
			// res := base.Cmd("create", "-i", "--network", network, "--mac-address", macAddress, testutil.CommonImage).Run()
			res := base.Cmd("create", "--network", network, "--name", containerName,
				"--mac-address", macAddress, testutil.CommonImage,
				"sh", "-c", "--", "ip addr show").Run()

			if !wantErr {
				assert.Assert(t, res.ExitCode == 0, "Command should have succeeded", res)
				// This is currently blocked by: https://github.com/containerd/nerdctl/pull/3104
				// res = base.Cmd("start", "-i", containerName).
				//	CmdOption(testutil.WithStdin(strings.NewReader("ip addr show eth0 | grep ether | awk '{printf $2}'"))).Run()
				res = base.Cmd("start", "-a", containerName).Run()
				// FIXME: flaky - this has failed on the CI once, with the output NOT containing anything
				// https://github.com/containerd/nerdctl/actions/runs/11392051487/job/31697214002?pr=3535#step:7:271
				assert.Assert(t, strings.Contains(res.Stdout(), expect), fmt.Sprintf("expected output to contain %q: %q", expect, res.Stdout()))
				assert.Assert(t, res.ExitCode == 0, "Command should have succeeded")
			} else {
				if testutil.GetTarget() == testutil.Docker &&
					(network == networkIPvlan || network == "container:whatever"+tID) {
					// unlike nerdctl
					// when using network ipvlan or container in Docker
					// it delays fail on executing start command
					assert.Assert(t, res.ExitCode == 0, "Command should have succeeded", res)
					res = base.Cmd("start", "-i", "-a", containerName).
						CmdOption(testutil.WithStdin(strings.NewReader("ip addr show eth0 | grep ether | awk '{printf $2}'"))).Run()
				}

				// See https://github.com/containerd/nerdctl/issues/3101
				if testutil.GetTarget() == testutil.Docker &&
					(network == networkBridge) {
					expect = ""
				}
				if expect != "" {
					assert.Assert(t, strings.Contains(res.Combined(), expect), fmt.Sprintf("expected output to contain %q: %q", expect, res.Combined()))
				} else {
					assert.Assert(t, res.Combined() == "", fmt.Sprintf("expected output to be empty: %q", res.Combined()))
				}
				assert.Assert(t, res.ExitCode != 0, "Command should have failed", res)
			}
		})
	}
}

func TestCreateWithTty(t *testing.T) {
	base := testutil.NewBase(t)
	imageName := testutil.CommonImage
	withoutTtyContainerName := "without-terminal-" + testutil.Identifier(t)
	withTtyContainerName := "with-terminal-" + testutil.Identifier(t)

	// without -t, fail
	base.Cmd("create", "--name", withoutTtyContainerName, imageName, "stty").AssertOK()
	base.Cmd("start", withoutTtyContainerName).AssertOK()
	defer base.Cmd("container", "rm", "-f", withoutTtyContainerName).AssertOK()
	base.Cmd("logs", withoutTtyContainerName).AssertCombinedOutContains("stty: standard input: Not a tty")
	withoutTtyContainer := base.InspectContainer(withoutTtyContainerName)
	assert.Equal(base.T, 1, withoutTtyContainer.State.ExitCode)

	// with -t, success
	base.Cmd("create", "-t", "--name", withTtyContainerName, imageName, "stty").AssertOK()
	base.Cmd("start", withTtyContainerName).AssertOK()
	defer base.Cmd("container", "rm", "-f", withTtyContainerName).AssertOK()
	base.Cmd("logs", withTtyContainerName).AssertCombinedOutContains("speed 38400 baud; line = 0;")
	withTtyContainer := base.InspectContainer(withTtyContainerName)
	assert.Equal(base.T, 0, withTtyContainer.State.ExitCode)
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
				dataRoot := data.TempDir()

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

				data.Set(containersPathKey, containersPath)
				data.Set(etchostsPathKey, etchostsPath)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "--data-root", data.TempDir(), "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--data-root", data.TempDir(), "--name", data.Identifier(), "-d", testutil.AlpineImage, "sleep", nerdtest.Infinity)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("is already used by ID")},
					Output: func(stdout string, info string, t *testing.T) {
						containersDirs, err := os.ReadDir(data.Get(containersPathKey))
						assert.NilError(t, err)
						assert.Equal(t, len(containersDirs), 1)

						etchostsDirs, err := os.ReadDir(data.Get(etchostsPathKey))
						assert.NilError(t, err)
						assert.Equal(t, len(etchostsDirs), 1)
					},
				}
			},
		},
		{
			Description: "Issue #2993 - nerdctl no longer leaks containers and etchosts directories and files when containers are removed.",
			Setup: func(data test.Data, helpers test.Helpers) {
				dataRoot := data.TempDir()

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

				data.Set(containersPathKey, containersPath)
				data.Set(etchostsPathKey, etchostsPath)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("--data-root", data.TempDir(), "rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("--data-root", data.TempDir(), "rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						containersDirs, err := os.ReadDir(data.Get(containersPathKey))
						assert.NilError(t, err)
						assert.Equal(t, len(containersDirs), 0)

						etchostsDirs, err := os.ReadDir(data.Get(etchostsPathKey))
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
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)

	// Docker does not support creating containers from OCI archive.
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	containerName := testutil.Identifier(t)

	teardown := func() {
		base.Cmd("rm", "-f", containerName).Run()
		base.Cmd("rmi", "-f", imageName).Run()
	}
	defer teardown()
	teardown()

	const sentinel = "test-nerdctl-create-from-oci-archive"
	dockerfile := fmt.Sprintf(`FROM %s
	CMD ["echo", "%s"]`, testutil.CommonImage, sentinel)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)
	tag := fmt.Sprintf("%s:latest", imageName)
	tarPath := fmt.Sprintf("%s/%s.tar", buildCtx, imageName)

	base.Cmd("build", "--tag", tag, fmt.Sprintf("--output=type=oci,dest=%s", tarPath), buildCtx).AssertOK()
	base.Cmd("create", "--rm", "--name", containerName, fmt.Sprintf("oci-archive://%s", tarPath)).AssertOK()
	base.Cmd("start", "--attach", containerName).AssertOutContains("test-nerdctl-create-from-oci-archive")
}
