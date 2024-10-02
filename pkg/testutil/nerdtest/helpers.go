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

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

const defaultNamespace = testutil.Namespace

// FIXME: unexport most of the following?
type target = string

const (
	targetNerdctl = target("nerdctl")
	TargetDocker  = target("docker")
)

func GetTarget() string {
	// Indirecting to testutil for now
	return testutil.GetTarget()
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
	maxRetry = 5
	sleep    = time.Second
)

func EnsureContainerStarted(helpers test.Helpers, con string) {
	for i := 0; i < maxRetry; i++ {
		count := i
		cmd := helpers.Command("container", "inspect", con)
		cmd.Run(&test.Expected{
			Output: func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Container
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				if dc[0].State.Running {
					return
				}
				if count == maxRetry-1 {
					t.Fatalf("container %s still not running after %d retries", con, count)
				}
				time.Sleep(sleep)
			},
		})
	}
}
