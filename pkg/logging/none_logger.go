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
	"context"

	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
)

type NoneLogger struct {
	Opts map[string]string
}

func (n *NoneLogger) Init(dataStore, ns, id string) error {
	return nil
}

func (n *NoneLogger) PreProcess(ctx context.Context, dataStore string, config *logging.Config) error {
	return nil
}

// Process is not reached: WriteLogEntry below makes this a SyncDriver, and
// loggingProcessAdapter only starts the Process goroutine for a driver that is
// not one. It must stay that way. Returning without draining the channels while
// the logger fed them would stall the container on its own stdout once the
// buffer filled.
func (n *NoneLogger) Process(stdout <-chan string, stderr <-chan string) error {
	return nil
}

// WriteLogEntry discards the entry.
//
// Implementing SyncDriver is what keeps this driver off the buffered channels
// in loggingProcessAdapter. Without it the logger would queue every line for a
// consumer that never reads, and a container producing more than the buffer
// holds would block on its own stdout.
func (n *NoneLogger) WriteLogEntry(stream, line string) error {
	return nil
}

func (n *NoneLogger) PostProcess() error {
	return nil
}

func NoneLogOptsValidate(_ map[string]string) error {
	return nil
}
