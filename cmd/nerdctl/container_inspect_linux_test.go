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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/docker/go-connections/nat"
	"gotest.tools/v3/assert"
)

func TestContainerInspectContainsPortConfig(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, "-p", "8080:80", testutil.NginxAlpineImage).AssertOK()
	inspect := base.InspectContainer(testContainer)
	inspect80TCP := (*inspect.NetworkSettings.Ports)["80/tcp"]
	expected := nat.PortBinding{
		HostIP:   "0.0.0.0",
		HostPort: "8080",
	}
	assert.Equal(base.T, expected, inspect80TCP[0])
}

func TestContainerInspectContainsMounts(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)

	testVolume := testutil.Identifier(t)

	defer base.Cmd("volume", "rm", "-f", testVolume).Run()
	base.Cmd("volume", "create", "--label", "tag=testVolume", testVolume).AssertOK()
	// inspectVolume := base.InspectVolume(testVolume)
	// namedVolumeSource := inspectVolume.Mountpoint

	defer base.Cmd("rm", "-f", testContainer).Run()
	base.Cmd("run", "-d", "--privileged",
		"--name", testContainer,
		"--network", "none",
		"-v", "/anony-vol",
		"--tmpfs", "/app1:size=64m",
		"--mount", "type=bind,src=/tmp,dst=/app2,ro",
		"--mount", fmt.Sprintf("type=volume,src=%s,dst=/app3,readonly=false", testVolume),
		testutil.NginxAlpineImage).AssertOK()

	inspect := base.InspectContainer(testContainer)

	// convert array to map to get by key of Destination
	actual := make(map[string]dockercompat.MountPoint)
	for i := range inspect.Mounts {
		actual[inspect.Mounts[i].Destination] = inspect.Mounts[i]
	}

	const localDriver = "local"

	expected := []struct {
		name       string
		dest       string
		mountPoint dockercompat.MountPoint
	}{
		// // anonymous volume
		// {
		// 	name: "anon volume test",
		// 	dest: "/anony-vol",
		// 	mountPoint: dockercompat.MountPoint{
		// 		Type:        "volume",
		// 		Name:        "",
		// 		Source:      "", // source of anonymous volume is a generated path, so here will not check it.
		// 		Destination: "/anony-vol",
		// 		Driver:      localDriver,
		// 		RW:          true,
		// 	},
		// },

		// bind
		{
			name: "bind mount test",
			dest: "/app2",
			mountPoint: dockercompat.MountPoint{
				Type:        "bind",
				Name:        "",
				Source:      "/tmp",
				Destination: "/app2",
				Driver:      "",
				RW:          false,
			},
		},

		// named volume
		// {
		// 	name: "named volume test",
		// 	dest: "/app3",
		// 	mountPoint: dockercompat.MountPoint{
		// 		Type:        "volume",
		// 		Name:        testVolume,
		// 		Source:      namedVolumeSource,
		// 		Destination: "/app3",
		// 		Driver:      localDriver,
		// 		RW:          true,
		// 	},
		// },
	}

	for i := range expected {
		testCase := expected[i]
		t.Logf("test volume[dest=%q]", testCase.dest)

		mountPoint, ok := actual[testCase.dest]
		assert.Assert(base.T, ok)

		assert.Equal(base.T, testCase.mountPoint.Type, mountPoint.Type)
		assert.Equal(base.T, testCase.mountPoint.Driver, mountPoint.Driver)
		assert.Equal(base.T, testCase.mountPoint.RW, mountPoint.RW)
		assert.Equal(base.T, testCase.mountPoint.Destination, mountPoint.Destination)

		if testCase.mountPoint.Source != "" {
			assert.Equal(base.T, testCase.mountPoint.Source, mountPoint.Source)
		}
		if testCase.mountPoint.Name != "" {
			assert.Equal(base.T, testCase.mountPoint.Name, mountPoint.Name)
		}
	}
}

func TestContainerInspectContainsLabel(t *testing.T) {
	t.Parallel()
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, "--label", "foo=foo", "--label", "bar=bar", testutil.NginxAlpineImage).AssertOK()
	base.EnsureContainerStarted(testContainer)
	inspect := base.InspectContainer(testContainer)
	lbs := inspect.Config.Labels

	assert.Equal(base.T, "foo", lbs["foo"])
	assert.Equal(base.T, "bar", lbs["bar"])
}

func TestContainerInspectState(t *testing.T) {
	t.Parallel()
	testContainer := testutil.Identifier(t)
	base := testutil.NewBase(t)

	type testCase struct {
		name, containerName, cmd string
		want                     dockercompat.ContainerState
	}
	// nerdctl: run error produces a nil Task, so the Status is empty because Status comes from Task.
	// docker : run error gives => `Status=created` as  in docker there is no a separation between container and Task.
	errStatus := ""
	if base.Target == testutil.Docker {
		errStatus = "created"
	}
	testCases := []testCase{
		{
			name:          "inspect State with error",
			containerName: fmt.Sprintf("%s-fail", testContainer),
			cmd:           "aa",
			want: dockercompat.ContainerState{
				Error:  "executable file not found in $PATH",
				Status: errStatus,
			},
		},
		{
			name:          "inspect State without error",
			containerName: fmt.Sprintf("%s-success", testContainer),
			cmd:           "ls",
			want: dockercompat.ContainerState{
				Error:  "",
				Status: "exited",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer base.Cmd("rm", "-f", tc.containerName).Run()
			if tc.want.Error != "" {
				base.Cmd("run", "--name", tc.containerName, testutil.AlpineImage, tc.cmd).AssertFail()
			} else {
				base.Cmd("run", "--name", tc.containerName, testutil.AlpineImage, tc.cmd).AssertOK()
			}
			inspect := base.InspectContainer(tc.containerName)
			assert.Assert(t, strings.Contains(inspect.State.Error, tc.want.Error), fmt.Sprintf("expected: %s, actual: %s", tc.want.Error, inspect.State.Error))
			assert.Equal(base.T, inspect.State.Status, tc.want.Status)
		})
	}

}
