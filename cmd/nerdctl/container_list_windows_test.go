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

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

type psTestContainer struct {
	name    string
	labels  map[string]string
	volumes []string
	network string
}

func preparePsTestContainer(t *testing.T, identity string, restart bool, hyperv bool) (*testutil.Base, psTestContainer) {
	base := testutil.NewBase(t)

	base.Cmd("pull", testutil.NginxAlpineImage).AssertOK()

	testContainerName := testutil.Identifier(t) + identity
	t.Cleanup(func() {
		base.Cmd("rm", "-f", testContainerName).AssertOK()
	})

	// A container can have multiple labels.
	// Therefore, this test container has multiple labels to check it.
	testLabels := make(map[string]string)
	keys := []string{
		testutil.Identifier(t) + identity,
		testutil.Identifier(t) + identity,
	}
	// fill the value of testLabels
	for _, k := range keys {
		testLabels[k] = k
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

	base.Cmd(args...).AssertOK()
	if restart {
		base.EnsureContainerStarted(testContainerName)
	}

	return base, psTestContainer{
		name:    testContainerName,
		labels:  testLabels,
		network: testContainerName,
	}
}

func TestListProcessContainer(t *testing.T) {
	base, testContainer := preparePsTestContainer(t, "list", true, false)

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
		assert.Equal(t, container, testContainer.name)

		image, _ := tab.ReadRow(lines[1], "IMAGE")
		assert.Equal(t, image, testutil.NginxAlpineImage)

		size, _ := tab.ReadRow(lines[1], "SIZE")

		// there is some difference between nerdctl and docker in calculating the size of the container
		expectedSize := "36.0 MiB (virtual "

		if !strings.Contains(size, expectedSize) {
			return fmt.Errorf("expect container size %s, but got %s", expectedSize, size)
		}

		return nil
	})
}

func TestListHyperVContainer(t *testing.T) {
	if !testutil.HyperVSupported() {
		t.Skip("HyperV is not enabled, skipping test")
	}

	base, testContainer := preparePsTestContainer(t, "list", true, true)
	inspect := base.InspectContainer(testContainer.name)
	//check with HCS if the container is ineed a VM
	isHypervContainer, err := testutil.HyperVContainer(inspect)
	if err != nil {
		t.Fatalf("unable to list HCS containers: %s", err)
	}
	assert.Assert(t, isHypervContainer, true)

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
		assert.Equal(t, container, testContainer.name)

		image, _ := tab.ReadRow(lines[1], "IMAGE")
		assert.Equal(t, image, testutil.NginxAlpineImage)

		size, _ := tab.ReadRow(lines[1], "SIZE")

		// there is some difference between nerdctl and docker in calculating the size of the container
		expectedSize := "72.0 MiB (virtual "

		if !strings.Contains(size, expectedSize) {
			return fmt.Errorf("expect container size %s, but got %s", expectedSize, size)
		}

		return nil
	})
}

func TestListProcessContainerWideMode(t *testing.T) {
	testutil.DockerIncompatible(t)
	base, testContainer := preparePsTestContainer(t, "listWithMode", true, false)

	// hope there are no tests running parallel
	base.Cmd("ps", "-n", "1", "--format", "wide").AssertOutWithFunc(func(stdout string) error {

		// An example of nerdctl ps --format wide
		// CONTAINER ID    IMAGE                               PLATFORM       COMMAND    CREATED              STATUS    PORTS    NAMES            RUNTIME                  SIZE
		// 17181f208b61    docker.io/library/busybox:latest    linux/amd64    "top"      About an hour ago    Up                 busybox-17181    io.containerd.runc.v2    16.0 KiB (virtual 1.3 MiB)

		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tRUNTIME\tPLATFORM\tSIZE")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}

		container, _ := tab.ReadRow(lines[1], "NAMES")
		assert.Equal(t, container, testContainer.name)

		image, _ := tab.ReadRow(lines[1], "IMAGE")
		assert.Equal(t, image, testutil.NginxAlpineImage)

		runtime, _ := tab.ReadRow(lines[1], "RUNTIME")
		assert.Equal(t, runtime, "io.containerd.runhcs.v1")

		size, _ := tab.ReadRow(lines[1], "SIZE")
		expectedSize := "36.0 MiB (virtual "
		if !strings.Contains(size, expectedSize) {
			return fmt.Errorf("expect container size %s, but got %s", expectedSize, size)
		}
		return nil
	})
}

func TestListProcessContainerWithLabels(t *testing.T) {
	base, testContainer := preparePsTestContainer(t, "listWithLabels", true, false)

	// hope there are no tests running parallel
	base.Cmd("ps", "-n", "1", "--format", "{{.Labels}}").AssertOutWithFunc(func(stdout string) error {

		// An example of nerdctl ps --format "{{.Labels}}"
		// key1=value1,key2=value2,key3=value3
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != 1 {
			return fmt.Errorf("expected 1 line, got %d", len(lines))
		}

		// check labels using map
		// 1. the results has no guarantee to show the same order.
		// 2. the results has no guarantee to show only configured labels.
		labelsMap, err := strutil.ParseCSVMap(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse labels: %v", err)
		}

		for i := range testContainer.labels {
			if value, ok := labelsMap[i]; ok {
				assert.Equal(t, value, testContainer.labels[i])
			}
		}
		return nil
	})
}
