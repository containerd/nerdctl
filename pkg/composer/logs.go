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

package composer

import (
	"io"
	"strings"

	"github.com/containerd/nerdctl/pkg/logging/pipetagger"
	"golang.org/x/sync/errgroup"

	"github.com/containerd/nerdctl/pkg/logging"
)

func (c *Composer) FormatLogs(containerName string, logsChan chan map[string]string, containerLogsEOFChan chan string, runEGTagger *errgroup.Group, lo logging.LogsOptions, rdStdout io.Reader, rdStderr io.Reader) error {
	var logTagMaxLen int
	logTag := strings.TrimPrefix(containerName, c.project.Name+"_")

	if l := len(logTag); l > logTagMaxLen {
		logTagMaxLen = l
	}

	logWidth := logTagMaxLen + 1
	if lo.NoLogPrefix {
		logWidth = -1
	}

	stdoutTagger := pipetagger.New(rdStdout, logTag, logWidth, lo.NoColor)
	stderrTagger := pipetagger.New(rdStderr, logTag, logWidth, lo.NoColor)

	for _, v := range []string{"stdout", "stderr"} {
		device := v
		runEGTagger.Go(func() error {
			switch device {
			case "stdout":
				stdoutTagger.Run(logsChan, device)
			case "stderr":
				stderrTagger.Run(logsChan, device)
			}
			containerLogsEOFChan <- containerName
			return nil
		})
	}

	return nil

}
