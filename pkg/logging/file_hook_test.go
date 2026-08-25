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

package logging

import (
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/log"
)

func TestFileHookFire(t *testing.T) {
	stamp := time.Date(2026, 4, 28, 3, 22, 52, 0, time.UTC)

	testCases := []struct {
		name     string
		entry    *log.Entry
		expected string
	}{
		{
			name: "message only",
			entry: &log.Entry{
				Time:    stamp,
				Level:   log.InfoLevel,
				Message: "creating container",
			},
			expected: `2026-04-28T03:22:52.000000000Z INFO creating container` + "\n",
		},
		{
			name: "fields are sorted",
			entry: &log.Entry{
				Time:    stamp,
				Level:   log.ErrorLevel,
				Message: "failed to create container",
				Data: log.Fields{
					"id":            "foo",
					"error":         errors.New("no such image"),
					"containerName": "bar",
				},
			},
			expected: `2026-04-28T03:22:52.000000000Z ERROR failed to create container ` +
				`containerName="bar" error="no such image" id="foo"` + "\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			hook := &fileHook{w: &sb}
			assert.NilError(t, hook.Fire(tc.entry))
			assert.Equal(t, sb.String(), tc.expected)
		})
	}
}

func TestFileHookLevels(t *testing.T) {
	// All levels must be accepted: entries are filtered against the logger level
	// before the hooks are fired.
	assert.Equal(t, len((&fileHook{}).Levels()), 7)
}

func TestSetLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "nerdctl.log")
	assert.NilError(t, os.WriteFile(logFile, []byte("previous invocation\n"), 0o600))

	savedHooks := log.L.Logger.ReplaceHooks(maps.Clone(log.L.Logger.Hooks))
	savedOut := log.L.Logger.Out
	t.Cleanup(func() {
		log.L.Logger.ReplaceHooks(savedHooks)
		log.L.Logger.SetOutput(savedOut)
	})

	closer, err := SetLogFile(logFile)
	assert.NilError(t, err)
	// Windows can not remove a file that is still open, and t.TempDir() cleans up
	// after this, so the handle has to go back first.
	t.Cleanup(func() { _ = closer.Close() })
	// The console output must be left alone, otherwise the formatter stops
	// detecting the terminal and downgrades its output style.
	assert.Equal(t, log.L.Logger.Out, savedOut)

	log.L.Logger.SetOutput(io.Discard)
	log.L.WithField("id", "foo").Error("failed to create container")

	b, err := os.ReadFile(logFile)
	assert.NilError(t, err)
	got := string(b)
	// Opened in append mode, so a concurrent or previous invocation is not lost.
	assert.Assert(t, strings.HasPrefix(got, "previous invocation\n"), got)
	assert.Assert(t, strings.Contains(got, `ERROR failed to create container id="foo"`), got)

	if runtime.GOOS != "windows" {
		st, err := os.Stat(logFile)
		assert.NilError(t, err)
		assert.Equal(t, st.Mode().Perm(), os.FileMode(0o600))
	}
}

func TestSetLogFileError(t *testing.T) {
	// A directory can not be opened for writing.
	_, err := SetLogFile(t.TempDir())
	assert.ErrorContains(t, err, "failed to open log file")
}
