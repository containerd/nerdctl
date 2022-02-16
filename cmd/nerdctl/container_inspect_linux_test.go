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

	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/testutil"
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
	inspectVolume := base.InspectVolume(testVolume)
	namedVolumeSource := inspectVolume.Mountpoint

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
		dest       string
		mountPoint dockercompat.MountPoint
	}{
		// anonymous volume
		{
			dest: "/anony-vol",
			mountPoint: dockercompat.MountPoint{
				Type:        "volume",
				Name:        "",
				Source:      "", // source of anonymous volume is a generated path, so here will not check it.
				Destination: "/anony-vol",
				Driver:      localDriver,
				RW:          true,
			},
		},

		// bind
		{
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
		{
			dest: "/app3",
			mountPoint: dockercompat.MountPoint{
				Type:        "volume",
				Name:        testVolume,
				Source:      namedVolumeSource,
				Destination: "/app3",
				Driver:      localDriver,
				RW:          true,
			},
		},
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
