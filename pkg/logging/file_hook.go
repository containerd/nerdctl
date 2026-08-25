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
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/containerd/log"
)

// fileHook mirrors nerdctl's own diagnostic log (not the container logs) to an
// additional writer.
//
// A hook is used rather than log.L.Logger.SetOutput(io.MultiWriter(...)) so that
// the console output keeps its current formatting: the formatter picks its output
// style by type-asserting Logger.Out to *os.File, which an io.MultiWriter is not.
type fileHook struct {
	mu sync.Mutex
	w  io.Writer
}

// Levels implements the logrus Hook interface. Entries are already filtered against
// the logger level before the hooks are fired, so all levels are accepted here.
func (h *fileHook) Levels() []log.Level {
	return []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
		log.DebugLevel,
		log.TraceLevel,
	}
}

// Fire implements the logrus Hook interface. The record format is deliberately
// independent of the console formatter, which varies with TTY detection.
func (h *fileHook) Fire(entry *log.Entry) error {
	var sb strings.Builder
	sb.WriteString(entry.Time.Format(log.RFC3339NanoFixed))
	sb.WriteString(" ")
	sb.WriteString(strings.ToUpper(entry.Level.String()))
	sb.WriteString(" ")
	sb.WriteString(entry.Message)
	for _, k := range slices.Sorted(maps.Keys(entry.Data)) {
		fmt.Fprintf(&sb, " %s=%q", k, fmt.Sprint(entry.Data[k]))
	}
	sb.WriteString("\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, sb.String())
	return err
}

// SetLogFile makes nerdctl append its own diagnostic log to path, in addition to
// the current output. The file is opened in append mode, so concurrent nerdctl
// invocations can share it.
//
// The returned io.Closer releases the file. The nerdctl CLI does not use it, as
// log.L.Fatal terminates the process and the hook writes are not buffered, but a
// library consumer has to be able to give the handle back.
func SetLogFile(path string) (io.Closer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %q: %w", path, err)
	}
	log.L.Logger.AddHook(&fileHook{w: f})
	return f, nil
}
