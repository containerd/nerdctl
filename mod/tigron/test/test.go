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

package test

import (
	"fmt"
	"os"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/highk"
)

// Testable TODO.
type Testable interface {
	CustomCommand(testCase *Case, t *testing.T) CustomizableCommand
	AmbientRequirements(testCase *Case, t *testing.T)
}

//nolint:gochecknoglobals // FIXME get rid of this
var registeredTestable Testable

// Customize TODO.
func Customize(testable Testable) {
	registeredTestable = testable
}

// LeakMainWrapper is to be called inside a TestMain function to enable goroutine and file descriptors leak detection.
func LeakMainWrapper(runner func() int) int {
	var exitCode int

	if err := highk.FindGoRoutines(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Leaking go routines")
		_, _ = fmt.Fprintln(os.Stderr, err.Error())

		return 1
	}

	var (
		snapFile      *os.File
		before, after []byte
	)

	if os.Getenv("HIGHK_EXPERIMENTAL_FD") != "" {
		snapFile, _ = os.CreateTemp("", "fileleaks")
		before, _ = highk.SnapshotOpenFiles(snapFile)
	}

	exitCode = runner()

	if exitCode != 0 {
		return exitCode
	}

	if os.Getenv("HIGHK_EXPERIMENTAL_FD") != "" {
		after, _ = highk.SnapshotOpenFiles(snapFile)
		diff := highk.Diff(string(before), string(after))

		if len(diff) != 0 {
			_, _ = fmt.Fprintln(os.Stderr, "Leaking file descriptors")

			for _, file := range diff {
				_, _ = fmt.Fprintln(os.Stderr, file)
			}

			exitCode = 1
		}
	}

	return exitCode
}
