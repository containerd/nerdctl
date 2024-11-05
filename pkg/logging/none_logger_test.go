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
	"os"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
)

func TestNoneLogger(t *testing.T) {
	// Create a temporary directory for potential log files
	tmpDir := t.TempDir()

	logger := &NoneLogger{
		Opts: map[string]string{},
	}

	t.Run("NoLoggingOccurs", func(t *testing.T) {
		initialFiles, err := os.ReadDir(tmpDir)
		assert.NilError(t, err, "Failed to read temp dir")

		// Run all logger methods
		logger.Init(tmpDir, "namespace", "id")
		logger.PreProcess(tmpDir, &logging.Config{})

		stdout := make(chan string)
		stderr := make(chan string)

		go func() {
			for i := 0; i < 10; i++ {
				stdout <- "test stdout"
				stderr <- "test stderr"
			}
			close(stdout)
			close(stderr)
		}()

		err = logger.Process(stdout, stderr)
		assert.NilError(t, err, "Process() returned unexpected error")

		logger.PostProcess()

		// Wait a bit to ensure any potential writes would have occurred
		time.Sleep(100 * time.Millisecond)

		// Check if any new files were created
		afterFiles, err := os.ReadDir(tmpDir)
		assert.NilError(t, err, "Failed to read temp dir after operations")

		assert.Equal(t, len(afterFiles), len(initialFiles), "Expected no new files, but directory content changed")

	})
}
