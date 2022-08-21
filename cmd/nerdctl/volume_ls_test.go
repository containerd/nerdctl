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

func TestVolumeLsFilter(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	var vol1, vol2, vol3, vol4 = tID + "vol-1", tID + "vol-2", tID + "vol-3", tID + "vol-4"
	var label1, label2, label3, label4 = tID + "=label-1", tID + "=label-2", tID + "=label-3", tID + "-group=label-4"
	base.Cmd("volume", "create", "--label="+label1, "--label="+label4, vol1).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol1).Run()

	base.Cmd("volume", "create", "--label="+label2, "--label="+label4, vol2).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol2).Run()

	base.Cmd("volume", "create", "--label="+label3, vol3).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol3).Run()

	base.Cmd("volume", "create", vol4).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol4).Run()

	base.Cmd("volume", "ls", "--quiet").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 4 {
			return errors.New("expected at least 4 lines")
		}
		volNames := map[string]struct{}{
			vol1: {},
			vol2: {},
			vol3: {},
			vol4: {},
		}

		var numMatches = 0
		for _, name := range lines {
			_, ok := volNames[name]
			if !ok {
				continue
			}
			numMatches++
		}
		if len(volNames) != numMatches {
			return fmt.Errorf("expected %d volumes, got: %d", len(volNames), numMatches)
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "label="+tID).AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 3 {
			return errors.New("expected at least 3 lines")
		}
		volNames := map[string]struct{}{
			vol1: {},
			vol2: {},
			vol3: {},
		}

		for _, name := range lines {
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "label="+label2).AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 1 {
			return errors.New("expected at least 1 lines")
		}
		volNames := map[string]struct{}{
			vol2: {},
		}

		for _, name := range lines {
			if name == "" {
				continue
			}
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "label="+tID+"=").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) > 0 {
			for _, name := range lines {
				if name != "" {
					return fmt.Errorf("unexpected volumes %d found", len(lines))
				}
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "label="+label1, "--filter", "label="+label2).AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) > 0 {
			for _, name := range lines {
				if name != "" {
					return fmt.Errorf("unexpected volumes %d found", len(lines))
				}
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "label="+tID, "--filter", "label="+label4).AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			return errors.New("expected at least 2 lines")
		}
		volNames := map[string]struct{}{
			vol1: {},
			vol2: {},
		}

		for _, name := range lines {
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "name="+vol1).AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 1 {
			return errors.New("expected at least 1 lines")
		}
		volNames := map[string]struct{}{
			vol1: {},
		}

		for _, name := range lines {
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "name=vol-3").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 1 {
			return errors.New("expected at least 1 lines")
		}
		volNames := map[string]struct{}{
			vol3: {},
		}

		for _, name := range lines {
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--quiet", "--filter", "name=vol2", "--filter", "name=vol1").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) > 0 {
			for _, name := range lines {
				if name != "" {
					return fmt.Errorf("unexpected volumes %d found", len(lines))
				}
			}
		}
		return nil
	})
}

func TestVolumeLsFilterSize(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	testutil.DockerIncompatible(t)

	var vol1, vol2, vol3, vol4 = tID + "volsize-1", tID + "volsize-2", tID + "volsize-3", tID + "volsize-4"
	var label1, label2, label3, label4 = tID + "=label-1", tID + "=label-2", tID + "=label-3", tID + "-group=label-4"
	base.Cmd("volume", "create", "--label="+label1, "--label="+label4, vol1).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol1).Run()

	base.Cmd("volume", "create", "--label="+label2, "--label="+label4, vol2).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol2).Run()

	base.Cmd("volume", "create", "--label="+label3, vol3).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol3).Run()

	base.Cmd("volume", "create", vol4).AssertOK()
	defer base.Cmd("volume", "rm", "-f", vol4).Run()

	createFileWithSize(t, vol1, 409600)
	createFileWithSize(t, vol2, 1024000)
	createFileWithSize(t, vol3, 409600)
	createFileWithSize(t, vol4, 1024000)

	base.Cmd("volume", "ls", "--size", "--filter", "size=1024000").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 3 {
			return errors.New("expected at least 3 lines")
		}

		var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
		var err = tab.ParseHeader(lines[0])
		if err != nil {
			return err
		}
		volNames := map[string]struct{}{
			vol2: {},
			vol4: {},
		}

		for _, line := range lines {
			name, _ := tab.ReadRow(line, "VOLUME NAME")
			if name == "VOLUME NAME" {
				continue
			}
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--size", "--filter", "size>=1024000", "--filter", "size<=2048000").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 3 {
			return errors.New("expected at least 3 lines")
		}

		var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
		var err = tab.ParseHeader(lines[0])
		if err != nil {
			return err
		}
		volNames := map[string]struct{}{
			vol2: {},
			vol4: {},
		}

		for _, line := range lines {
			name, _ := tab.ReadRow(line, "VOLUME NAME")
			if name == "VOLUME NAME" {
				continue
			}
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("volume", "ls", "--size", "--filter", "size>204800", "--filter", "size<1024000").AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 3 {
			return errors.New("expected at least 3 lines")
		}

		var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
		var err = tab.ParseHeader(lines[0])
		if err != nil {
			return err
		}
		volNames := map[string]struct{}{
			vol1: {},
			vol3: {},
		}

		for _, line := range lines {
			name, _ := tab.ReadRow(line, "VOLUME NAME")
			if name == "VOLUME NAME" {
				continue
			}
			_, ok := volNames[name]
			if !ok {
				return fmt.Errorf("unexpected volume %s found", name)
			}
		}
		return nil
	})
}
