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
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"gotest.tools/v3/assert"
)

func setupPsTestContainer(identity string, restart bool, hyperv bool) func(data test.Data, helpers test.Helpers) {
	return func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)
		containerName := data.Identifier(identity)
		data.Labels().Set("containerName", containerName)
		// A container can have multiple labels.
		testLabels := make(map[string]string)
		for i := 0; i < 2; i++ {
			k := fmt.Sprintf("%s-%d", data.Identifier(identity), i)
			testLabels[k] = k
			data.Labels().Set(fmt.Sprintf("label-key-%d", i), k)
			data.Labels().Set(fmt.Sprintf("label-value-%d", i), k)
		}
		args := []string{
			"run",
			"-d",
		}
		if hyperv {
			args = append(args, "--isolation", "hyperv")
		}
		args = append(args,
			"--name", containerName,
			"--label", formatter.FormatLabels(testLabels),
			testutil.NginxAlpineImage,
		)
		if !restart {
			args = append(args, "--restart=no")
		}
		helpers.Ensure(args...)
		if restart {
			nerdtest.EnsureContainerStarted(helpers, containerName)
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
				assert.Assert(t, strings.Contains(size, "(virtual"),
					fmt.Sprintf("expect container size to contain '(virtual', but got %s", size))
			},
		}
	}
	testCase.Run(t)
}
func TestListHyperVContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.HyperV
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer("list", true, true)(data, helpers)
		containerName := data.Labels().Get("containerName")
		inspect := nerdtest.InspectContainer(helpers, containerName)
		isHypervContainer, err := testutil.HyperVContainer(inspect)
		assert.NilError(helpers.T(), err, "unable to list HCS containers")
		assert.Assert(helpers.T(), isHypervContainer, "expected HyperV container")
	}
	testCase.Cleanup = cleanupPsTestContainer()
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-s", "--filter", fmt.Sprintf("name=%s", data.Labels().Get("containerName")))
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
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
		return helpers.Command("ps", "--format", "wide", "--filter", fmt.Sprintf("name=%s", data.Labels().Get("containerName")))
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
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
		return helpers.Command("ps", "--format", "{{.Labels}}", "--filter", fmt.Sprintf("name=%s", data.Labels().Get("containerName")))
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) == 1, fmt.Sprintf("expected 1 line, got %d", len(lines)))
				labelsMap, err := strutil.ParseCSVMap(lines[0])
				assert.NilError(t, err, "failed to parse labels")
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
