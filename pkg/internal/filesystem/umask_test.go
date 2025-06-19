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

package filesystem_test

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
)

func TestUmask(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("windows does not have a unix-style umask")
	}

	userHostReportedUmask, err := exec.Command("sh", "-c", "umask").CombinedOutput()
	assert.NilError(t, err, fmt.Sprintf(
		"umask command should succeed (output: %s)",
		userHostReportedUmask,
	))
	expectedUmask, err := strconv.ParseInt(strings.TrimSpace(string(userHostReportedUmask)), 8, 0)
	assert.NilError(
		t,
		err,
		fmt.Sprintf("umask command should have returned parsable output (was: %s)", userHostReportedUmask),
	)

	userMask := filesystem.GetUmask()
	assert.Equal(t, expectedUmask, int64(userMask), "system reported umask and implementation umask are the same")

	userHostReportedUmask, err = exec.Command("sh", "-c", "umask").CombinedOutput()
	assert.NilError(t, err)
	expectedUmask, err = strconv.ParseInt(strings.TrimSpace(string(userHostReportedUmask)), 8, 0)
	assert.NilError(t, err)

	assert.Equal(t, expectedUmask, int64(userMask), "system reported umask has not changed")
}

func TestUmaskConcurrent(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("windows does not have a unix-style umask")
	}

	userHostReportedUmask, err := exec.Command("sh", "-c", "umask").Output()
	assert.NilError(t, err)
	expectedUmask, err := strconv.ParseInt(strings.TrimSpace(string(userHostReportedUmask)), 8, 0)
	assert.NilError(t, err)

	var counter int32 = 100

	ch := make(chan uint32)

	for range counter {
		go func(ch chan uint32) {
			u := filesystem.GetUmask()
			if atomic.AddInt32(&counter, -1) == 0 {
				ch <- u
			}
		}(ch)
	}

	ret := <-ch
	assert.Equal(t, expectedUmask, int64(ret))
	userHostReportedUmask, err = exec.Command("sh", "-c", "umask").Output()
	assert.NilError(t, err)
	newUmask, err := strconv.ParseInt(strings.TrimSpace(string(userHostReportedUmask)), 8, 0)
	assert.NilError(t, err)
	assert.Equal(t, newUmask, expectedUmask, "system reported umask has not changed")
}
