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
	"errors"
	"fmt"
	"os"
	"slices"
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

// When keepAlive is false, the container will exit immediately with status 1.
func preparePsTestContainer(t *testing.T, identity string, keepAlive bool) (*testutil.Base, psTestContainer) {
	base := testutil.NewBase(t)

	base.Cmd("pull", testutil.CommonImage).AssertOK()

	testContainerName := testutil.Identifier(t) + identity
	rwVolName := testContainerName + "-rw"
	// A container can mount named and anonymous volumes
	rwDir, err := os.MkdirTemp(t.TempDir(), "rw")
	if err != nil {
		t.Fatal(err)
	}
	base.Cmd("network", "create", testContainerName).AssertOK()
	t.Cleanup(func() {
		base.Cmd("rm", "-f", testContainerName).AssertOK()
		base.Cmd("volume", "rm", "-f", rwVolName).Run()
		base.Cmd("network", "rm", testContainerName).Run()
		os.RemoveAll(rwDir)
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
	base.Cmd("volume", "create", rwVolName).AssertOK()
	mnt1 := fmt.Sprintf("%s:/%s_mnt1", rwDir, identity)
	mnt2 := fmt.Sprintf("%s:/%s_mnt3", rwVolName, identity)

	args := []string{
		"run",
		"-d",
		"--name",
		testContainerName,
		"--label",
		formatter.FormatLabels(testLabels),
		"-v", mnt1,
		"-v", mnt2,
		"--net", testContainerName,
	}
	if keepAlive {
		args = append(args, testutil.CommonImage, "top")
	} else {
		args = append(args, "--restart=no", testutil.CommonImage, "false")
	}

	base.Cmd(args...).AssertOK()
	if keepAlive {
		base.EnsureContainerStarted(testContainerName)
	} else {
		base.EnsureContainerExited(testContainerName, 1)
	}

	// dd if=/dev/zero of=test_file bs=1M count=25
	// let the container occupy 25MiB space.
	if keepAlive {
		base.Cmd("exec", testContainerName, "dd", "if=/dev/zero", "of=/test_file", "bs=1M", "count=25").AssertOK()
	}
	volumes := []string{}
	volumes = append(volumes, strings.Split(mnt1, ":")...)
	volumes = append(volumes, strings.Split(mnt2, ":")...)

	return base, psTestContainer{
		name:    testContainerName,
		labels:  testLabels,
		volumes: volumes,
		network: testContainerName,
	}
}

func TestContainerList(t *testing.T) {
	base, testContainer := preparePsTestContainer(t, "list", true)

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
	base, testContainer := preparePsTestContainer(t, "listWithMode", true)

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

func TestContainerListWithLabels(t *testing.T) {
	base, testContainer := preparePsTestContainer(t, "listWithLabels", true)

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

func TestContainerListWithNames(t *testing.T) {
	base, testContainer := preparePsTestContainer(t, "listWithNames", true)

	// hope there are no tests running parallel
	base.Cmd("ps", "-n", "1", "--format", "{{.Names}}").AssertOutWithFunc(func(stdout string) error {

		// An example of nerdctl ps --format "{{.Names}}"
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != 1 {
			return fmt.Errorf("expected 1 line, got %d", len(lines))
		}

		assert.Equal(t, lines[0], testContainer.name)

		return nil
	})
}

func TestContainerListWithFilter(t *testing.T) {
	base, testContainerA := preparePsTestContainer(t, "listWithFilterA", true)
	_, testContainerB := preparePsTestContainer(t, "listWithFilterB", true)
	_, testContainerC := preparePsTestContainer(t, "listWithFilterC", false)

	base.Cmd("ps", "--filter", "name="+testContainerA.name).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}

		containerName, _ := tab.ReadRow(lines[1], "NAMES")
		assert.Equal(t, containerName, testContainerA.name)
		id, _ := tab.ReadRow(lines[1], "CONTAINER ID")
		base.Cmd("ps", "-q", "--filter", "id="+id).AssertOutWithFunc(func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) != 1 {
				return fmt.Errorf("expected 1 line, got %d", len(lines))
			}
			if lines[0] != id {
				return errors.New("failed to filter by id")
			}
			return nil
		})
		base.Cmd("ps", "-q", "--filter", "id="+id+id).AssertOutWithFunc(func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) > 0 {
				for _, line := range lines {
					if line != "" {
						return fmt.Errorf("unexpected container found: %s", line)
					}
				}
			}
			return nil
		})
		base.Cmd("ps", "-q", "--filter", "id=").AssertOutWithFunc(func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) > 0 {
				for _, line := range lines {
					if line != "" {
						return fmt.Errorf("unexpected container found: %s", line)
					}
				}
			}
			return nil
		})
		return nil
	})

	base.Cmd("ps", "-q", "--filter", "name="+testContainerA.name+testContainerA.name).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) > 0 {
			for _, line := range lines {
				if line != "" {
					return fmt.Errorf("unexpected container found: %s", line)
				}
			}
		}
		return nil
	})

	base.Cmd("ps", "-q", "--filter", "name=").AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) == 0 {
			return errors.New("expect at least 1 container, got 0")
		}
		return nil
	})

	base.Cmd("ps", "--filter", "name=listWithFilter").AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 3 {
			return fmt.Errorf("expected at least 3 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		containerNames := map[string]struct{}{
			testContainerA.name: {}, testContainerB.name: {},
		}
		for idx, line := range lines {
			if idx == 0 {
				continue
			}
			containerName, _ := tab.ReadRow(line, "NAMES")
			if _, ok := containerNames[containerName]; !ok {
				return fmt.Errorf("unexpected container %s found", containerName)
			}
		}
		return nil
	})

	// docker filter by id only support full ID no truncate
	// https://github.com/docker/for-linux/issues/258
	// yet nerdctl also support truncate ID
	base.Cmd("ps", "--no-trunc", "--filter", "since="+testContainerA.name).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		var id string
		for idx, line := range lines {
			if idx == 0 {
				continue
			}
			containerName, _ := tab.ReadRow(line, "NAMES")
			if containerName != testContainerB.name {
				return fmt.Errorf("unexpected container %s found", containerName)
			}
			id, _ = tab.ReadRow(line, "CONTAINER ID")
		}
		base.Cmd("ps", "--filter", "before="+id).AssertOutWithFunc(func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) < 2 {
				return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
			}

			tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
			err := tab.ParseHeader(lines[0])
			if err != nil {
				return fmt.Errorf("failed to parse header: %v", err)
			}
			foundA := false
			for idx, line := range lines {
				if idx == 0 {
					continue
				}
				containerName, _ := tab.ReadRow(line, "NAMES")
				if containerName == testContainerA.name {
					foundA = true
					break
				}
			}
			// there are other containers such as **wordpress** could be listed since
			// their created times are ahead of testContainerB too
			if !foundA {
				return fmt.Errorf("expected container %s not found", testContainerA.name)
			}
			return nil
		})
		return nil
	})

	// docker filter by id only support full ID no truncate
	// https://github.com/docker/for-linux/issues/258
	// yet nerdctl also support truncate ID
	base.Cmd("ps", "--no-trunc", "--filter", "before="+testContainerB.name).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		foundA := false
		var id string
		for idx, line := range lines {
			if idx == 0 {
				continue
			}
			containerName, _ := tab.ReadRow(line, "NAMES")
			if containerName == testContainerA.name {
				foundA = true
				id, _ = tab.ReadRow(line, "CONTAINER ID")
				break
			}
		}
		// there are other containers such as **wordpress** could be listed since
		// their created times are ahead of testContainerB too
		if !foundA {
			return fmt.Errorf("expected container %s not found", testContainerA.name)
		}
		base.Cmd("ps", "--filter", "since="+id).AssertOutWithFunc(func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) < 2 {
				return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
			}

			tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
			err := tab.ParseHeader(lines[0])
			if err != nil {
				return fmt.Errorf("failed to parse header: %v", err)
			}
			for idx, line := range lines {
				if idx == 0 {
					continue
				}
				containerName, _ := tab.ReadRow(line, "NAMES")
				if containerName != testContainerB.name {
					return fmt.Errorf("unexpected container %s found", containerName)
				}
			}
			return nil
		})
		return nil
	})

	for _, testContainer := range []psTestContainer{testContainerA, testContainerB} {
		for _, volume := range testContainer.volumes {
			base.Cmd("ps", "--filter", "volume="+volume).AssertOutWithFunc(func(stdout string) error {
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				if len(lines) < 2 {
					return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
				}

				tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
				err := tab.ParseHeader(lines[0])
				if err != nil {
					return fmt.Errorf("failed to parse header: %v", err)
				}
				containerName, _ := tab.ReadRow(lines[1], "NAMES")
				assert.Equal(t, containerName, testContainer.name)
				return nil
			})
		}
	}

	base.Cmd("ps", "--filter", "network="+testContainerA.network).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		containerName, _ := tab.ReadRow(lines[1], "NAMES")
		assert.Equal(t, containerName, testContainerA.name)
		return nil
	})

	for key, value := range testContainerB.labels {
		base.Cmd("ps", "--filter", "label="+key+"="+value).AssertOutWithFunc(func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) < 2 {
				return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
			}

			tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
			err := tab.ParseHeader(lines[0])
			if err != nil {
				return fmt.Errorf("failed to parse header: %v", err)
			}
			containerNames := map[string]struct{}{
				testContainerB.name: {},
			}
			for idx, line := range lines {
				if idx == 0 {
					continue
				}
				containerName, _ := tab.ReadRow(line, "NAMES")
				if _, ok := containerNames[containerName]; !ok {
					return fmt.Errorf("unexpected container %s found", containerName)
				}
			}
			return nil
		})
	}

	base.Cmd("ps", "-a", "--filter", "exited=1").AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		containerNames := map[string]struct{}{
			testContainerC.name: {},
		}
		for idx, line := range lines {
			if idx == 0 {
				continue
			}
			containerName, _ := tab.ReadRow(line, "NAMES")
			if _, ok := containerNames[containerName]; !ok {
				return fmt.Errorf("unexpected container %s found", containerName)
			}
		}
		return nil
	})

	base.Cmd("ps", "-a", "--filter", "status=exited").AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
		}

		tab := tabutil.NewReader("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		err := tab.ParseHeader(lines[0])
		if err != nil {
			return fmt.Errorf("failed to parse header: %v", err)
		}
		containerNames := map[string]struct{}{
			testContainerC.name: {},
		}
		for idx, line := range lines {
			if idx == 0 {
				continue
			}
			containerName, _ := tab.ReadRow(line, "NAMES")
			if _, ok := containerNames[containerName]; !ok {
				return fmt.Errorf("unexpected container %s found", containerName)
			}
		}
		return nil
	})
}

func TestContainerListCheckCreatedTime(t *testing.T) {
	base, _ := preparePsTestContainer(t, "checkCreatedTimeA", true)
	preparePsTestContainer(t, "checkCreatedTimeB", true)
	preparePsTestContainer(t, "checkCreatedTimeC", false)
	preparePsTestContainer(t, "checkCreatedTimeD", false)

	var createdTimes []string

	base.Cmd("ps", "--format", "'{{json .CreatedAt}}'", "-a").AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 4 {
			return fmt.Errorf("expected at least 4 lines, got %d", len(lines))
		}
		createdTimes = append(createdTimes, lines...)
		return nil
	})

	slices.Reverse(createdTimes)
	if !slices.IsSorted(createdTimes) {
		t.Errorf("expected containers in decending order")
	}
}
