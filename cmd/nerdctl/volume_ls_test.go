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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/tabutil"
	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestVolumeLs(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	testutil.DockerIncompatible(t)

	var vol1, vol2, vol3 = tID + "vol-1", tID + "vol-2", tID + "empty"
	base.Cmd("volume", "create", vol1).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol1).Run()

	base.Cmd("volume", "create", vol2).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol2).Run()

	base.Cmd("volume", "create", vol3).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol3).Run()

	createFileWithSize(t, vol1, 102400)
	createFileWithSize(t, vol2, 204800)

	base.Cmd("volume", "ls", "--size").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 4 {
			return errors.New("expected at least 4 lines")
		}
		volSizes := map[string]string{
			vol1: "100.0 KiB",
			vol2: "200.0 KiB",
			vol3: "0.0 B",
		}

		var numMatches = 0
		var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
		var err = tab.ParseHeader(lines[0])
		if err != nil {
			return err
		}
		for _, line := range lines {
			name, _ := tab.ReadRow(line, "VOLUME NAME")
			size, _ := tab.ReadRow(line, "SIZE")
			expectSize, ok := volSizes[name]
			if !ok {
				continue
			}
			if size != expectSize {
				return fmt.Errorf("expected size %s for volume %s, got %s", expectSize, name, size)
			}
			numMatches++
		}
		if len(volSizes) != numMatches {
			return fmt.Errorf("expected %d volumes, got: %d", len(volSizes), numMatches)
		}
		return nil
	})

}
