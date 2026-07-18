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
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// setupPsTestContainer creates a test container with labels, volumes, and network.
// When keepAlive is false, the container exits immediately with status 1.
// Container info is stored in data.Labels() keyed by identity prefix:
//   - container-{identity}: container name
//   - network-{identity}: network name (same as container)
//   - vol-{identity}: named volume name
//   - labelkey-{identity}: label key
//   - labelval-{identity}: label value
//   - volume-{identity}-{0..3}: volume mount components for filter tests
func setupPsTestContainer(data test.Data, helpers test.Helpers, identity string, keepAlive bool) {
	containerName := data.Identifier(identity)
	rwVolName := containerName + "-rw"
	rwDir := data.Temp().Dir(identity + "-rw")

	helpers.Ensure("pull", "--quiet", testutil.CommonImage)
	helpers.Ensure("network", "create", containerName)
	helpers.Ensure("volume", "create", rwVolName)

	// A container can have multiple labels.
	// Therefore, this test container has labels to check.
	testLabels := make(map[string]string)
	testLabels[containerName] = containerName

	mnt1 := fmt.Sprintf("%s:/%s_mnt1", rwDir, identity)
	mnt2 := fmt.Sprintf("%s:/%s_mnt3", rwVolName, identity)

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--label", formatter.FormatLabels(testLabels),
		"-v", mnt1,
		"-v", mnt2,
		"--net", containerName,
	}
	if keepAlive {
		args = append(args, testutil.CommonImage, "top")
	} else {
		args = append(args, "--restart=no", testutil.CommonImage, "false")
	}

	helpers.Ensure(args...)
	if keepAlive {
		nerdtest.EnsureContainerStarted(helpers, containerName)
		// dd if=/dev/zero of=test_file bs=1M count=25
		// let the container occupy 25MiB space.
		helpers.Ensure("exec", containerName, "dd", "if=/dev/zero", "of=/test_file", "bs=1M", "count=25")
	} else {
		nerdtest.EnsureContainerExited(helpers, containerName, 1)
	}

	data.Labels().Set("container-"+identity, containerName)
	data.Labels().Set("network-"+identity, containerName)
	data.Labels().Set("vol-"+identity, rwVolName)
	data.Labels().Set("labelkey-"+identity, containerName)
	data.Labels().Set("labelval-"+identity, containerName)

	volumes := []string{}
	volumes = append(volumes, strings.Split(mnt1, ":")...)
	volumes = append(volumes, strings.Split(mnt2, ":")...)
	for i, v := range volumes {
		data.Labels().Set(fmt.Sprintf("volume-%s-%d", identity, i), v)
	}
}

// cleanupPsTestContainer removes the container, volume, and network created by setupPsTestContainer.
func cleanupPsTestContainer(data test.Data, helpers test.Helpers, identity string) {
	containerName := data.Identifier(identity)
	helpers.Anyhow("rm", "-f", containerName)
	helpers.Anyhow("volume", "rm", "-f", containerName+"-rw")
	helpers.Anyhow("network", "rm", containerName)
}

func TestContainerList(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer(data, helpers, "list", true)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanupPsTestContainer(data, helpers, "list")
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-n", "1", "-s")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				containerName := data.Labels().Get("container-list")

				// An example of nerdctl/docker ps -n 1 -s
				// CONTAINER ID    IMAGE                               COMMAND    CREATED           STATUS    PORTS    NAMES            SIZE
				// be8d386c991e    docker.io/library/busybox:latest    "top"      1 second ago    Up                 c1       16.0 KiB (virtual 1.3 MiB)
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

				tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tSIZE")
				err := tab.ParseHeader(lines[0])
				assert.NilError(t, err, "failed to parse header")

				container, _ := tab.ReadRow(lines[1], "NAMES")
				assert.Equal(t, container, containerName)

				image, _ := tab.ReadRow(lines[1], "IMAGE")
				assert.Equal(t, image, testutil.CommonImage)

				size, _ := tab.ReadRow(lines[1], "SIZE")
				// there is some difference between nerdctl and docker in calculating the size of the container
				expectedSize := "26.2MB (virtual "
				if !nerdtest.IsDocker() {
					expectedSize = "25.0 MiB (virtual "
				}
				assert.Assert(t, strings.Contains(size, expectedSize),
					"expect container size %s, but got %s", expectedSize, size)
			},
		}
	}
	testCase.Run(t)
}

func TestContainerListWideMode(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer(data, helpers, "listWithMode", true)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanupPsTestContainer(data, helpers, "listWithMode")
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-n", "1", "--format", "wide")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				containerName := data.Labels().Get("container-listWithMode")

				// An example of nerdctl ps --format wide
				// CONTAINER ID    IMAGE                               PLATFORM       COMMAND    CREATED              STATUS    PORTS    NAMES            RUNTIME                  SIZE
				// 17181f208b61    docker.io/library/busybox:latest    linux/amd64    "top"      About an hour ago    Up                 busybox-17181    io.containerd.runc.v2    16.0 KiB (virtual 1.3 MiB)
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

				tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tRUNTIME\tPLATFORM\tSIZE")
				err := tab.ParseHeader(lines[0])
				assert.NilError(t, err, "failed to parse header")

				container, _ := tab.ReadRow(lines[1], "NAMES")
				assert.Equal(t, container, containerName)

				image, _ := tab.ReadRow(lines[1], "IMAGE")
				assert.Equal(t, image, testutil.CommonImage)

				runtime, _ := tab.ReadRow(lines[1], "RUNTIME")
				assert.Equal(t, runtime, "io.containerd.runc.v2")

				size, _ := tab.ReadRow(lines[1], "SIZE")
				expectedSize := "25.0 MiB (virtual "
				assert.Assert(t, strings.Contains(size, expectedSize),
					"expect container size %s, but got %s", expectedSize, size)
			},
		}
	}
	testCase.Run(t)
}

func TestContainerListWithLabels(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer(data, helpers, "listWithLabels", true)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanupPsTestContainer(data, helpers, "listWithLabels")
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-n", "1", "--format", "{{.Labels}}")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				// An example of nerdctl ps --format "{{.Labels}}"
				// key1=value1,key2=value2,key3=value3
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Equal(t, len(lines), 1, "expected 1 line")

				// check labels using map
				// 1. the results has no guarantee to show the same order.
				// 2. the results has no guarantee to show only configured labels.
				labelsMap, err := strutil.ParseCSVMap(lines[0])
				assert.NilError(t, err, "failed to parse labels")

				labelKey := data.Labels().Get("labelkey-listWithLabels")
				labelVal := data.Labels().Get("labelval-listWithLabels")
				if value, ok := labelsMap[labelKey]; ok {
					assert.Equal(t, value, labelVal)
				}
			},
		}
	}
	testCase.Run(t)
}

func TestContainerListWithNames(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer(data, helpers, "listWithNames", true)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanupPsTestContainer(data, helpers, "listWithNames")
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "-n", "1", "--format", "{{.Names}}")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				containerName := data.Labels().Get("container-listWithNames")

				// An example of nerdctl ps --format "{{.Names}}"
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Equal(t, len(lines), 1, "expected 1 line")
				assert.Equal(t, lines[0], containerName)
			},
		}
	}
	testCase.Run(t)
}

func TestContainerListWithFilter(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer(data, helpers, "A", true)
		setupPsTestContainer(data, helpers, "B", true)
		setupPsTestContainer(data, helpers, "C", false)

		containerA := data.Labels().Get("container-A")
		containerB := data.Labels().Get("container-B")

		ctrA := nerdtest.InspectContainer(helpers, containerA)
		data.Labels().Set("fullIdA", ctrA.ID)
		data.Labels().Set("shortIdA", ctrA.ID[:12])

		ctrB := nerdtest.InspectContainer(helpers, containerB)
		data.Labels().Set("fullIdB", ctrB.ID)

		commonLen := 0
		for commonLen < len(containerA) && commonLen < len(containerB) {
			if containerA[commonLen] != containerB[commonLen] {
				break
			}
			commonLen++
		}
		data.Labels().Set("commonPrefix", strings.TrimRight(containerA[:commonLen], "-"))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanupPsTestContainer(data, helpers, "A")
		cleanupPsTestContainer(data, helpers, "B")
		cleanupPsTestContainer(data, helpers, "C")
	}
	testCase.SubTests = containerListFilterSubTests()
	testCase.Run(t)
}

func containerListFilterSubTests() []*test.Case {
	return []*test.Case{
		{
			Description: "filter by name shows correct container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "name="+data.Labels().Get("container-A"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerA := data.Labels().Get("container-A")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						containerName, _ := tab.ReadRow(lines[1], "NAMES")
						assert.Equal(t, containerName, containerA)
					},
				}
			},
		},
		{
			Description: "filter by truncated id",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-q", "--filter", "id="+data.Labels().Get("shortIdA"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						shortID := data.Labels().Get("shortIdA")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Equal(t, len(lines), 1, fmt.Sprintf("expected 1 line, got %d", len(lines)))
						assert.Equal(t, lines[0], shortID)
					},
				}
			},
		},
		{
			Description: "filter by doubled id returns empty",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				id := data.Labels().Get("shortIdA")
				return helpers.Command("ps", "-q", "--filter", "id="+id+id)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
							assert.Equal(t, line, "", "unexpected container found: "+line)
						}
					},
				}
			},
		},
		{
			Description: "filter by empty id returns empty",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-q", "--filter", "id=")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
							assert.Equal(t, line, "", "unexpected container found: "+line)
						}
					},
				}
			},
		},
		{
			Description: "filter by name regexp",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerA := data.Labels().Get("container-A")
				return helpers.Command("ps", "--filter", "name=.*"+containerA+".*")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerA := data.Labels().Get("container-A")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						containerName, _ := tab.ReadRow(lines[1], "NAMES")
						assert.Equal(t, containerName, containerA)
					},
				}
			},
		},
		{
			Description: "filter by name anchored regexp",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerA := data.Labels().Get("container-A")
				return helpers.Command("ps", "--filter", "name=^"+containerA+"$")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerA := data.Labels().Get("container-A")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						containerName, _ := tab.ReadRow(lines[1], "NAMES")
						assert.Equal(t, containerName, containerA)
					},
				}
			},
		},
		{
			Description: "filter by doubled name returns empty",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerA := data.Labels().Get("container-A")
				return helpers.Command("ps", "-q", "--filter", "name="+containerA+containerA)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
							assert.Equal(t, line, "", "unexpected container found: "+line)
						}
					},
				}
			},
		},
		{
			Description: "filter by empty name returns all",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-q", "--filter", "name=")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 1, "expect at least 1 container, got 0")
					},
				}
			},
		},
		{
			Description: "filter by partial name shows multiple containers",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "name="+data.Labels().Get("commonPrefix"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerA := data.Labels().Get("container-A")
						containerB := data.Labels().Get("container-B")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 3, "expected at least 3 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						containerNames := map[string]struct{}{
							containerA: {}, containerB: {},
						}
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							containerName, _ := tab.ReadRow(line, "NAMES")
							_, ok := containerNames[containerName]
							assert.Assert(t, ok, "unexpected container %s found", containerName)
						}
					},
				}
			},
		},
		// docker filter by id only support full ID no truncate
		// https://github.com/docker/for-linux/issues/258
		// yet nerdctl also support truncate ID
		{
			Description: "filter since by name shows only later container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--no-trunc", "--filter", "since="+data.Labels().Get("container-A"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerB := data.Labels().Get("container-B")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							name, _ := tab.ReadRow(line, "NAMES")
							assert.Equal(t, name, containerB, "unexpected container found")
						}
					},
				}
			},
		},
		{
			Description: "filter before by full id includes earlier container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "before="+data.Labels().Get("fullIdB"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerA := data.Labels().Get("container-A")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						// there are other containers that could be listed since
						// their created times are ahead of containerB too
						foundA := false
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							name, _ := tab.ReadRow(line, "NAMES")
							if name == containerA {
								foundA = true
								break
							}
						}
						assert.Assert(t, foundA, "expected container %s not found", containerA)
					},
				}
			},
		},
		{
			Description: "filter before by name includes earlier container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--no-trunc", "--filter", "before="+data.Labels().Get("container-B"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerA := data.Labels().Get("container-A")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						// there are other containers that could be listed since
						// their created times are ahead of containerB too
						foundA := false
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							name, _ := tab.ReadRow(line, "NAMES")
							if name == containerA {
								foundA = true
								break
							}
						}
						assert.Assert(t, foundA, "expected container %s not found", containerA)
					},
				}
			},
		},
		{
			Description: "filter since by full id shows only later container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "since="+data.Labels().Get("fullIdA"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerB := data.Labels().Get("container-B")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							name, _ := tab.ReadRow(line, "NAMES")
							assert.Equal(t, name, containerB, "unexpected container found")
						}
					},
				}
			},
		},
		{
			Description: "filter by volume",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				for _, identity := range []string{"A", "B"} {
					containerName := data.Labels().Get("container-" + identity)
					for i := 0; i < 4; i++ {
						vol := data.Labels().Get(fmt.Sprintf("volume-%s-%d", identity, i))
						helpers.Command("ps", "--filter", "volume="+vol).
							Run(&test.Expected{
								Output: func(stdout string, t tig.T) {
									lines := strings.Split(strings.TrimSpace(stdout), "\n")
									assert.Assert(t, len(lines) >= 2,
										"expected at least 2 lines for volume=%s, got %d", vol, len(lines))

									tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
									err := tab.ParseHeader(lines[0])
									assert.NilError(t, err)
									name, _ := tab.ReadRow(lines[1], "NAMES")
									assert.Equal(t, name, containerName)
								},
							})
					}
				}
			},
		},
		{
			Description: "filter by network",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "network="+data.Labels().Get("network-A"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerA := data.Labels().Get("container-A")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						containerName, _ := tab.ReadRow(lines[1], "NAMES")
						assert.Equal(t, containerName, containerA)
					},
				}
			},
		},
		{
			Description: "filter by label",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				labelKey := data.Labels().Get("labelkey-B")
				labelVal := data.Labels().Get("labelval-B")
				return helpers.Command("ps", "--filter", "label="+labelKey+"="+labelVal)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerB := data.Labels().Get("container-B")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							containerName, _ := tab.ReadRow(line, "NAMES")
							assert.Equal(t, containerName, containerB, "unexpected container found")
						}
					},
				}
			},
		},
		{
			Description: "filter by exited with -a",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--filter", "exited=1")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerC := data.Labels().Get("container-C")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							containerName, _ := tab.ReadRow(line, "NAMES")
							assert.Equal(t, containerName, containerC, "unexpected container found")
						}
					},
				}
			},
		},
		{
			Description: "filter by status=exited with -a",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--filter", "status=exited")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerC := data.Labels().Get("container-C")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							containerName, _ := tab.ReadRow(line, "NAMES")
							assert.Equal(t, containerName, containerC, "unexpected container found")
						}
					},
				}
			},
		},
		{
			Description: "filter by status=exited without -a",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "status=exited")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						containerC := data.Labels().Get("container-C")
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines, got %d", len(lines))

						tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
						err := tab.ParseHeader(lines[0])
						assert.NilError(t, err)
						for idx, line := range lines {
							if idx == 0 {
								continue
							}
							containerName, _ := tab.ReadRow(line, "NAMES")
							assert.Equal(t, containerName, containerC, "unexpected container found")
						}
					},
				}
			},
		},
	}
}

func TestContainerListCheckCreatedTime(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		setupPsTestContainer(data, helpers, "checkCreatedTimeA", true)
		setupPsTestContainer(data, helpers, "checkCreatedTimeB", true)
		setupPsTestContainer(data, helpers, "checkCreatedTimeC", false)
		setupPsTestContainer(data, helpers, "checkCreatedTimeD", false)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanupPsTestContainer(data, helpers, "checkCreatedTimeA")
		cleanupPsTestContainer(data, helpers, "checkCreatedTimeB")
		cleanupPsTestContainer(data, helpers, "checkCreatedTimeC")
		cleanupPsTestContainer(data, helpers, "checkCreatedTimeD")
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("ps", "--format", "'{{json .CreatedAt}}'", "-a")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				assert.Assert(t, len(lines) >= 4, "expected at least 4 lines, got %d", len(lines))

				reversed := make([]string, len(lines))
				copy(reversed, lines)
				slices.Reverse(reversed)
				assert.Assert(t, slices.IsSorted(reversed), "expected containers in descending order")
			},
		}
	}
	testCase.Run(t)
}

func TestContainerListStatusFilter(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("create", "--name", data.Identifier("container"), testutil.CommonImage, "echo", "foo")
		data.Labels().Set("cID", data.Identifier("container"))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
	}

	testCase.SubTests = []*test.Case{
		// TODO: Refactor other filter tests
		{
			Description: "ps filter with status=created",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--filter", "status=created", "--filter", fmt.Sprintf("name=%s", data.Labels().Get("cID")))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, data.Labels().Get("cID")), "No container found with status created")
					},
				}
			},
		},
	}

	testCase.Run(t)
}

func TestContainerListWithHealthStatus(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		// Docker CLI does not provide a standalone healthcheck command.
		require.Not(nerdtest.Docker),
		require.Not(nerdtest.Rootless),
	)

	testCase.SubTests = []*test.Case{
		{
			Description: "ps shows healthy status after a successful probe",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "true", "--health-interval", "3s",
					testutil.CommonImage, "sleep", nerdtest.Infinity,
				)
				helpers.Ensure("container", "healthcheck", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "name="+data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Contains(fmt.Sprintf("(%s)", healthcheck.Healthy)),
				}
			},
		},
		{
			Description: "ps shows starting status",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "exit 1",
					"--health-interval", "1s",
					"--health-start-period", "60s",
					"--health-retries", "2",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("container", "healthcheck", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "name="+data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Contains(fmt.Sprintf("(health: %s)", healthcheck.Starting)),
				}
			},
		},
		{
			Description: "ps shows unhealthy status",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "not-a-real-cmd",
					"--health-interval", "1s",
					"--health-retries", "1",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("container", "healthcheck", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "--filter", "name="+data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Contains(fmt.Sprintf("(%s)", healthcheck.Unhealthy)),
				}
			},
		},
		{
			Description: "ps does not show health suffix for stopped containers",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "true",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("container", "healthcheck", data.Identifier())
				helpers.Ensure("stop", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--filter", "name="+data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Contains("Exited"),
						expect.DoesNotContain(
							"(health:",
							fmt.Sprintf("(%s)", healthcheck.Healthy),
							fmt.Sprintf("(%s)", healthcheck.Unhealthy),
						),
					),
				}
			},
		},
	}

	testCase.Run(t)
}
