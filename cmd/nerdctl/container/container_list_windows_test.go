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
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func setupPsTestContainer(identity string, restart bool, hyperv bool) func(data test.Data, helpers test.Helpers) {
	return func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)

		testContainerName := data.Identifier(identity)
		data.Labels().Set("containerName", testContainerName)

		// A container can have multiple labels.
		// Therefore, this test container has multiple labels to check it.
		testLabels := make(map[string]string)
		keys := []string{
			data.Identifier(identity),
			data.Identifier(identity),
		}
		// fill the value of testLabels
		for idx, k := range keys {
			testLabels[k] = k
			data.Labels().Set(fmt.Sprintf("label-key-%d", idx), k)
			data.Labels().Set(fmt.Sprintf("label-value-%d", idx), k)
		}

		args := []string{
			"run",
			"-d",
			"--name",
			testContainerName,
			"--label",
			formatter.FormatLabels(testLabels),
			testutil.NginxAlpineImage,
		}
		if !restart {
			args = append(args, "--restart=no")
		}
		if hyperv {
			args = append(args[:3], args[1:]...)
			args[1], args[2] = "--isolation", "hyperv"
		}

		helpers.Ensure(args...)
		if restart {
			// Wait for container to start - using polling
			helpers.Ensure("exec", testContainerName, "echo", "ready")
		}
	}
}

func cleanupPsTestContainer() func(data test.Data, helpers test.Helpers) {
	return func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("containerName"))
	}
}

func TestListProcessContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = setupPsTestContainer("list", true, false)
	testCase.Cleanup = cleanupPsTestContainer()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-s", "--filter", fmt.Sprintf("name=%s", data.Labels().Get("containerName")))
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				// An example of nerdctl/docker ps -n 1 -s
				// CONTAINER ID    IMAGE                               COMMAND    CREATED           STATUS    PORTS    NAMES            SIZE
				// be8d386c991e    docker.io/library/busybox:latest    "top"      1 second ago    Up                 c1       16.0 KiB (virtual 1.3 MiB)

				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) >= 2, fmt.Sprintf("expected at least 2 lines, got %d", len(lines)))

				tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tSIZE")
				err := tab.ParseHeader(lines[0])
				assert.NilError(t, err, "failed to parse header")

				container, _ := tab.ReadRow(lines[1], "NAMES")
				assert.Equal(t, container, data.Labels().Get("containerName"))

				image, _ := tab.ReadRow(lines[1], "IMAGE")
				assert.Equal(t, image, testutil.NginxAlpineImage)

				size, _ := tab.ReadRow(lines[1], "SIZE")

				// there is some difference between nerdctl and docker in calculating the size of the container
				// nerdctl: "36.0 MiB (virtual ...)"
				// docker: "53.2kB (virtual 19.3MB)" or similar
				assert.Assert(t, strings.Contains(size, "(virtual"),
					fmt.Sprintf("expect container size to contain '(virtual', but got %s", size))
			},
		}
	}

	testCase.Run(t)
}

func TestListHyperVContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = &test.Requirement{
		Check: func(_ test.Data, _ test.Helpers) (bool, string) {
			if !testutil.HyperVSupported() {
				return false, "HyperV is not enabled, skipping test"
			}
			return true, ""
		},
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer("list", true, true)(data, helpers)

		// Check with HCS if the container is indeed a VM
		containerName := data.Labels().Get("containerName")
		inspectOutput := helpers.Capture("inspect", containerName, "--format", "json")

		var inspect []dockercompat.Container
		err := json.Unmarshal([]byte(inspectOutput), &inspect)
		assert.NilError(helpers.T(), err, "failed to unmarshal inspect output")
		assert.Assert(helpers.T(), len(inspect) > 0, "expected at least one container in inspect output")

		isHypervContainer, err := testutil.HyperVContainer(inspect[0])
		assert.NilError(helpers.T(), err, "unable to list HCS containers")
		assert.Assert(helpers.T(), isHypervContainer, "expected HyperV container")
	}
	testCase.Cleanup = cleanupPsTestContainer()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-n", "1", "-s")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				// An example of nerdctl/docker ps -n 1 -s
				// CONTAINER ID    IMAGE                               COMMAND    CREATED           STATUS    PORTS    NAMES            SIZE
				// be8d386c991e    docker.io/library/busybox:latest    "top"      1 second ago    Up                 c1       16.0 KiB (virtual 1.3 MiB)

				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) >= 2, fmt.Sprintf("expected at least 2 lines, got %d", len(lines)))

				tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tSIZE")
				err := tab.ParseHeader(lines[0])
				assert.NilError(t, err, "failed to parse header")

				container, _ := tab.ReadRow(lines[1], "NAMES")
				assert.Equal(t, container, data.Labels().Get("containerName"))

				image, _ := tab.ReadRow(lines[1], "IMAGE")
				assert.Equal(t, image, testutil.NginxAlpineImage)

				size, _ := tab.ReadRow(lines[1], "SIZE")

				// there is some difference between nerdctl and docker in calculating the size of the container
				expectedSize := "72.0 MiB (virtual "

				assert.Assert(t, strings.Contains(size, expectedSize),
					fmt.Sprintf("expect container size %s, but got %s", expectedSize, size))
			},
		}
	}

	testCase.Run(t)
}

func TestListProcessContainerWideMode(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = setupPsTestContainer("listWithMode", true, false)
	testCase.Cleanup = cleanupPsTestContainer()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-n", "1", "--format", "wide")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				// An example of nerdctl ps --format wide
				// CONTAINER ID    IMAGE                               PLATFORM       COMMAND    CREATED              STATUS    PORTS    NAMES            RUNTIME                  SIZE
				// 17181f208b61    docker.io/library/busybox:latest    linux/amd64    "top"      About an hour ago    Up                 busybox-17181    io.containerd.runc.v2    16.0 KiB (virtual 1.3 MiB)

				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) >= 2, fmt.Sprintf("expected at least 2 lines, got %d", len(lines)))

				tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tRUNTIME\tPLATFORM\tSIZE")
				err := tab.ParseHeader(lines[0])
				assert.NilError(t, err, "failed to parse header")

				container, _ := tab.ReadRow(lines[1], "NAMES")
				assert.Equal(t, container, data.Labels().Get("containerName"))

				image, _ := tab.ReadRow(lines[1], "IMAGE")
				assert.Equal(t, image, testutil.NginxAlpineImage)

				runtime, _ := tab.ReadRow(lines[1], "RUNTIME")
				assert.Equal(t, runtime, "io.containerd.runhcs.v1")

				size, _ := tab.ReadRow(lines[1], "SIZE")
				expectedSize := "36.0 MiB (virtual "
				assert.Assert(t, strings.Contains(size, expectedSize),
					fmt.Sprintf("expect container size %s, but got %s", expectedSize, size))
			},
		}
	}

	testCase.Run(t)
}

func TestListProcessContainerWithLabels(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = setupPsTestContainer("listWithLabels", true, false)
	testCase.Cleanup = cleanupPsTestContainer()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-n", "1", "--format", "{{.Labels}}")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				// An example of nerdctl ps --format "{{.Labels}}"
				// key1=value1,key2=value2,key3=value3
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) == 1, fmt.Sprintf("expected 1 line, got %d", len(lines)))

				// check labels using map
				// 1. the results has no guarantee to show the same order.
				// 2. the results has no guarantee to show only configured labels.
				labelsMap, err := strutil.ParseCSVMap(lines[0])
				assert.NilError(t, err, "failed to parse labels")

				// Verify the labels we set are present
				for idx := 0; idx < 2; idx++ {
					labelKey := data.Labels().Get(fmt.Sprintf("label-key-%d", idx))
					labelValue := data.Labels().Get(fmt.Sprintf("label-value-%d", idx))
					if value, ok := labelsMap[labelKey]; ok {
						assert.Equal(t, value, labelValue)
					}
				}
			},
		}
	}

	testCase.Run(t)
}
