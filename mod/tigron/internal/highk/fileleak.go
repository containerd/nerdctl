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

package highk

import (
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// FIXME: it seems that lsof (or go test) is interfering and showing false positive KQUEUE / inodes
//
//nolint:gochecknoglobals // FIXME rewrite all of this anyhow
var whitelist = map[string]bool{
	"KQUEUE":  true,
	"a_inode": true,
}

// SnapshotOpenFiles will capture the list of currently open-files for the process.
//
//nolint:wrapcheck // FIXME: work in progress
func SnapshotOpenFiles(file *os.File) ([]byte, error) {
	// Using a buffer would add a pipe to the list of files.
	// Reimplement this stuff in go ASAP and toss lsof instead of passing around fd.
	_, _ = file.Seek(0, 0)
	_ = file.Truncate(0)

	exe, err := exec.LookPath("lsof")
	if err != nil {
		return nil, err
	}

	//nolint:gosec // G204 is fine here
	cmd := exec.Command(exe, "-nP", "-p", strconv.Itoa(syscall.Getpid()))
	cmd.Stdout = file

	_ = cmd.Run()

	_, _ = file.Seek(0, 0)

	return io.ReadAll(file)
}

// Diff will return a slice of strings showing the diff between two strings.
func Diff(one, two string) []string {
	aone := strings.Split(one, "\n")
	atwo := strings.Split(two, "\n")

	slices.Sort(aone)
	slices.Sort(atwo)

	loss := make(map[string]bool, len(aone))
	gain := map[string]bool{}

	for _, v := range aone {
		loss[v] = true
	}

	for _, v := range atwo {
		if _, ok := loss[v]; ok {
			delete(loss, v)
		} else {
			gain[v] = true
		}
	}

	diff := []string{}

	for key := range loss {
		legit := true

		for wl := range whitelist {
			if strings.Contains(key, wl) {
				legit = false
			}
		}

		if legit {
			diff = append(diff, "- "+key)
		}
	}

	for key := range gain {
		legit := true

		for wl := range whitelist {
			if strings.Contains(key, wl) {
				legit = false
			}
		}

		if legit {
			diff = append(diff, "+ "+key)
		}
	}

	return diff
}
