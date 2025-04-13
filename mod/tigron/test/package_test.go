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

package test_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/highk"
)

func TestMain(m *testing.M) {
	// Prep exit code
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	var (
		snapFile      *os.File
		before, after []byte
	)

	if os.Getenv("HIGHK_EXPERIMENTAL_FD") != "" {
		snapFile, _ = os.CreateTemp("", "fileleaks")
		before, _ = highk.SnapshotOpenFiles(snapFile)
	}

	exitCode = m.Run()

	if exitCode != 0 {
		return
	}

	if os.Getenv("HIGHK_EXPERIMENTAL_FD") != "" {
		after, _ = highk.SnapshotOpenFiles(snapFile)
		diff := highk.Diff(string(before), string(after))

		if len(diff) != 0 {
			fmt.Fprintln(os.Stderr, "Leaking file descriptors")

			for _, file := range diff {
				fmt.Fprintln(os.Stderr, file)
			}

			exitCode = 1
		}
	}

	if err := highk.FindGoRoutines(); err != nil {
		fmt.Fprintln(os.Stderr, "Leaking go routines")
		fmt.Fprintln(os.Stderr, os.Stderr, err.Error())

		exitCode = 1
	}
}
