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

package nerdtest

import (
	"encoding/json"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

const (
	// It seems that at this moment, the busybox on windows image we are using has an outdated version of sleep
	// that does not support inf/infinity.
	// This constant is provided as a mean for tests to express the intention of sleep infinity without having to
	// worry about that and get windows compatibility.
	Infinity = "3600"
)

func IsDocker() bool {
	return testutil.GetTarget() == "docker"
}

// InspectContainer is a helper that can be used inside custom commands or Setup
func InspectContainer(helpers test.Helpers, name string) dockercompat.Container {
	var dc []dockercompat.Container
	cmd := helpers.Command("container", "inspect", name)
	cmd.Run(&test.Expected{
		Output: func(stdout string, info string, t *testing.T) {
			err := json.Unmarshal([]byte(stdout), &dc)
			assert.NilError(t, err, "Unable to unmarshal output\n"+info)
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
		},
	})
	return dc[0]
}

func InspectVolume(helpers test.Helpers, name string) native.Volume {
	var dc []native.Volume
	cmd := helpers.Command("volume", "inspect", name)
	cmd.Run(&test.Expected{
		Output: func(stdout string, info string, t *testing.T) {
			err := json.Unmarshal([]byte(stdout), &dc)
			assert.NilError(t, err, "Unable to unmarshal output\n"+info)
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
		},
	})
	return dc[0]
}

func InspectNetwork(helpers test.Helpers, name string) dockercompat.Network {
	var dc []dockercompat.Network
	cmd := helpers.Command("network", "inspect", name)
	cmd.Run(&test.Expected{
		Output: func(stdout string, info string, t *testing.T) {
			err := json.Unmarshal([]byte(stdout), &dc)
			assert.NilError(t, err, "Unable to unmarshal output\n"+info)
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
		},
	})
	return dc[0]
}

func InspectImage(helpers test.Helpers, name string) dockercompat.Image {
	var dc []dockercompat.Image
	cmd := helpers.Command("image", "inspect", name)
	cmd.Run(&test.Expected{
		Output: func(stdout string, info string, t *testing.T) {
			err := json.Unmarshal([]byte(stdout), &dc)
			assert.NilError(t, err, "Unable to unmarshal output\n"+info)
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
		},
	})
	return dc[0]
}

const (
	maxRetry = 20
	sleep    = time.Second
)

func EnsureContainerStarted(helpers test.Helpers, con string) {
	started := false
	for i := 0; i < maxRetry && !started; i++ {
		helpers.Command("container", "inspect", con).
			Run(&test.Expected{
				ExitCode: expect.ExitCodeNoCheck,
				Output: func(stdout string, info string, t *testing.T) {
					var dc []dockercompat.Container
					err := json.Unmarshal([]byte(stdout), &dc)
					if err != nil || len(dc) == 0 {
						return
					}
					assert.Equal(t, len(dc), 1, "Unexpectedly got multiple results\n"+info)
					started = dc[0].State.Running
				},
			})
		time.Sleep(sleep)
	}

	if !started {
		ins := helpers.Capture("container", "inspect", con)
		lgs := helpers.Capture("logs", con)
		ps := helpers.Capture("ps", "-a")
		helpers.T().Log(ins)
		helpers.T().Log(lgs)
		helpers.T().Log(ps)
		helpers.T().Fatalf("container %s still not running after %d retries", con, maxRetry)
	}
}
