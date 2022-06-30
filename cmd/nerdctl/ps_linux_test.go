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

	"github.com/containerd/nerdctl/pkg/tabutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func prepareTest(t *testing.T) (*testutil.Base, string) {
	base := testutil.NewBase(t)

	base.Cmd("pull", testutil.CommonImage).AssertOK()

	testContainerName := testutil.Identifier(t)
	t.Cleanup(func() {
		base.Cmd("rm", "-f", testContainerName).AssertOK()
	})

	base.Cmd([]string{
		"run",
		"-d",
		"--name",
		testContainerName,
		testutil.CommonImage,
		"top",
	}...).AssertOK()

	// dd if=/dev/zero of=test_file bs=1M count=25
	// let the container occupy 25MiB space.
	base.Cmd("exec", testContainerName, "dd", "if=/dev/zero", "of=/test_file", "bs=1M", "count=25").AssertOK()

	return base, testContainerName
}

func TestContainerList(t *testing.T) {
	base, testContainerName := prepareTest(t)

	// hope there are no tests running parallel
	base.Cmd("ps", "-n", "1", "-s").AssertOutWithFunc(func(stdout string) error {
		// An example of nerdctl/docker ps -n 1 -s
		// CONTAINER ID    IMAGE                               COMMAND    CREATED           STATUS    PORTS    NAMES            SIZE
		// be8d386c991e    docker.io/library/busybox:latest    "top"      1 second ago    Up                 c1       16.0 KiB (virtual 1.3 MiB)

		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tSIZE")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}

		container, _ := tab.ReadRow(lines[1], "NAMES")
		assert.Equal(t, container, testContainerName)

		image, _ := tab.ReadRow(lines[1], "IMAGE")
		assert.Equal(t, image, testutil.CommonImage)

		size, _ := tab.ReadRow(lines[1], "SIZE")

		// there is some difference between nerdctl and docker in calculating the size of the container
		expectedSize := "26.2MB (virtual "
		if base.Target != testutil.Docker {
			expectedSize = "25.0 MiB (virtual "
		}

		if !strings.Contains(size, expectedSize) {
			return fmt.Errorf("expect container size %s, but got %s", expectedSize, size)
		}

		return nil
	})
}

func TestContainerListWideMode(t *testing.T) {
	testutil.DockerIncompatible(t)
	base, testContainerName := prepareTest(t)

	// hope there are no tests running parallel
	base.Cmd("ps", "-n", "1", "--format", "wide").AssertOutWithFunc(func(stdout string) error {

		// An example of nerdctl ps --format wide
		// CONTAINER ID    IMAGE                               PLATFORM       COMMAND    CREATED              STATUS    PORTS    NAMES            RUNTIME                  SIZE
		// 17181f208b61    docker.io/library/busybox:latest    linux/amd64    "top"      About an hour ago    Up                 busybox-17181    io.containerd.runc.v2    16.0 KiB (virtual 1.3 MiB)

		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tPLATFORM\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tRUNTIME\tSIZE")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}

		container, _ := tab.ReadRow(lines[1], "NAMES")
		assert.Equal(t, container, testContainerName)

		image, _ := tab.ReadRow(lines[1], "IMAGE")
		assert.Equal(t, image, testutil.CommonImage)

		runtime, _ := tab.ReadRow(lines[1], "RUNTIME")
		assert.Equal(t, runtime, "io.containerd.runc.v2")

		size, _ := tab.ReadRow(lines[1], "SIZE")
		expectedSize := "25.0 MiB (virtual "
		if !strings.Contains(size, expectedSize) {
			return fmt.Errorf("expect container size %s, but got %s", expectedSize, size)
		}
		return nil
	})
}
