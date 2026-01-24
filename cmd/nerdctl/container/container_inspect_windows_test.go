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
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestInspectProcessContainerContainsLabel(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		containerName := testutil.Identifier(t)
		data.Labels().Set("containerName", containerName)
		helpers.Ensure("run", "-d", "--name", containerName, "--label", "foo=foo", "--label", "bar=bar", testutil.CommonImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, containerName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("rm", "-f", containerName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		containerName := data.Labels().Get("containerName")
		return helpers.Command("inspect", containerName)
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err)
				assert.Equal(t, 1, len(dc))

				assert.Equal(t, "foo", dc[0].Config.Labels["foo"])
				assert.Equal(t, "bar", dc[0].Config.Labels["bar"])
			},
		}
	}

	testCase.Run(t)
}

func TestInspectHyperVContainerContainsLabel(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.HyperV

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		containerName := testutil.Identifier(t)
		data.Labels().Set("containerName", containerName)
		helpers.Ensure("run", "-d", "--name", containerName, "--isolation", "hyperv", "--label", "foo=foo", "--label", "bar=bar", testutil.CommonImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, containerName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("rm", "-f", containerName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		containerName := data.Labels().Get("containerName")
		return helpers.Command("inspect", containerName)
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err)
				assert.Equal(t, 1, len(dc))

				//check with HCS if the container is ineed a VM
				isHypervContainer, err := testutil.HyperVContainer(dc[0])
				assert.NilError(t, err)
				assert.Equal(t, true, isHypervContainer)

				assert.Equal(t, "foo", dc[0].Config.Labels["foo"])
				assert.Equal(t, "bar", dc[0].Config.Labels["bar"])
			},
		}
	}

	testCase.Run(t)
}
