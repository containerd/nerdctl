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

package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// TODO: Optimize health status reads/writes to avoid excessive file I/O.
// Currently, each health check and every container inspect operation reads/writes the entire health log file.
// Consider keeping health state, failure streak in labels instead of persisting to disk. logs can still be stored in disk and read during inspect.
// This will improve performance, especially for containers with frequent health checks or when many concurrent inspect calls are made.

var mu sync.Mutex

// writeHealthLog writes a health check result to the log file.
func writeHealthLog(ctx context.Context, container containerd.Container, result *Health) error {
	mu.Lock()
	defer mu.Unlock()

	stateDir, err := getContainerStateDir(ctx, container)
	if err != nil {
		fmt.Printf("Error fetching container state dir: %v\n", err)
		return err
	}

	// Ensure file exists before writing
	if err := ensureHealthLogFile(stateDir); err != nil {
		return err
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal health log: %w", err)
	}

	path := filepath.Join(stateDir, HealthLogFilename)

	return os.WriteFile(path, data, 0o600)
}

// readHealthLog reads the entire health state from the health.json file, including all logs.
func readHealthLog(ctx context.Context, container containerd.Container) (*Health, error) {
	stateDir, err := getContainerStateDir(ctx, container)
	if err != nil {
		fmt.Printf("Error fetching container state dir: %v\n", err)
		return nil, err
	}

	path := filepath.Join(stateDir, HealthLogFilename)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var h Health
	if err := json.NewDecoder(f).Decode(&h); err != nil {
		return nil, err
	}
	return &h, nil
}

func readHealth(stateDir string) (*Health, error) {
	path := filepath.Join(stateDir, HealthLogFilename)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var h Health
	if err := json.NewDecoder(f).Decode(&h); err != nil {
		return nil, err
	}
	return &h, nil
}

// ReadHealthLogForInspect reads the health state for container inspect, truncating logs and outputs as needed.
func ReadHealthLogForInspect(stateDir string) (*Health, error) {
	h, err := readHealth(stateDir)
	if err != nil || h == nil {
		return h, err // propagate nil or error
	}

	// Truncate to the most recent MaxLogEntries
	if len(h.Log) > MaxLogEntries {
		h.Log = h.Log[:MaxLogEntries]
	}
	// Truncate each log output using limitedBuffer
	for _, logEntry := range h.Log {
		if len(logEntry.Output) > MaxOutputLenForInspect {
			lb := NewResizableBuffer(MaxOutputLenForInspect) // Mimic docker's 4K limit
			_, _ = lb.Write([]byte(logEntry.Output))
			logEntry.Output = lb.String()
		}
	}
	return h, nil
}

// ensureHealthLogFile creates the health.json file if it doesn't exist.
func ensureHealthLogFile(stateDir string) error {
	healthLogPath := filepath.Join(stateDir, HealthLogFilename)

	// Ensure container state directory exists
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		return fmt.Errorf("container state directory does not exist: %s", stateDir)
	}

	// Create health.json if it doesn't exist
	if _, err := os.Stat(healthLogPath); os.IsNotExist(err) {
		return os.WriteFile(healthLogPath, []byte{}, 0600)
	}

	return nil
}

// getContainerStateDir returns the container's state directory from labels.
func getContainerStateDir(ctx context.Context, container containerd.Container) (string, error) {
	info, err := container.Info(ctx)
	if err != nil {
		return "", err
	}
	stateDir, ok := info.Labels[labels.StateDir]
	if !ok {
		return "", err
	}
	return stateDir, nil
}

// ResizableBuffer collects output with a configurable upper limit.
type ResizableBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	maxSize   int
	truncated bool
}

// NewResizableBuffer returns a new buffer with the given size limit in bytes.
func NewResizableBuffer(maxSize int) *ResizableBuffer {
	return &ResizableBuffer{maxSize: maxSize}
}

func (b *ResizableBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	remaining := b.maxSize - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}

	if len(p) > remaining {
		b.truncated = true
		p = p[:remaining]
	}

	return b.buf.Write(p)
}

func (b *ResizableBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := b.buf.String()
	if b.truncated {
		s += "... [truncated]"
	}
	return s
}
