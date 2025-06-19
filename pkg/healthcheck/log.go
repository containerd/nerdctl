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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// writeHealthLog writes the latest health check result to the log file, appending it to existing logs.
func writeHealthLog(ctx context.Context, container containerd.Container, result *HealthcheckResult) error {
	stateDir, err := getContainerStateDir(ctx, container)
	if err != nil {
		return fmt.Errorf("error fetching container state dir: %v", err)
	}

	data, err := result.ToJSONString()
	if err != nil {
		return fmt.Errorf("failed to marshal health log: %w", err)
	}

	// Ensure file exists before writing
	if err := ensureHealthLogFile(stateDir); err != nil {
		return err
	}

	// Write the latest result to the file
	logPath := filepath.Join(stateDir, HealthLogFilename)
	return filesystem.WithAppendLock(logPath, func(file *os.File) error {
		if _, err := file.Seek(0, io.SeekEnd); err != nil {
			return fmt.Errorf("seek error: %w", err)
		}
		if _, err := file.Write(append([]byte(data), '\n')); err != nil {
			return fmt.Errorf("failed to write health log: %w", err)
		}
		return nil
	})
}

// ReadHealthStatusForInspect reads the health state from labels and the last MaxLogEntries health check result logs.
func ReadHealthStatusForInspect(stateDir, healthState string) (*Health, error) {
	state, err := HealthStateFromJSON(healthState)
	if err != nil {
		return nil, fmt.Errorf("failed to parse health state: %w", err)
	}

	logPath := filepath.Join(stateDir, HealthLogFilename)
	var logs []*HealthcheckResult
	err = filesystem.WithReadOnlyLock(logPath, func() error {
		file, err := os.Open(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		defer file.Close()

		reader := bufio.NewReader(file)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}

			line = strings.TrimRight(line, "\n")
			result, err := HealthcheckResultFromJSON(line)
			if err != nil {
				log.L.Warnf("failed to parse healthcheck log line: %v", err)
				continue
			}
			logs = append(logs, result)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Keep only the last MaxLogEntries
	n := len(logs)
	if n > MaxLogEntries {
		logs = logs[n-MaxLogEntries:]
	}

	// Reverse for newest-first order
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}

	// Truncate log outputs to avoid flooding inspect output
	for _, logEntry := range logs {
		if len(logEntry.Output) > MaxOutputLenForInspect {
			buf := NewResizableBuffer(MaxOutputLenForInspect)
			_, _ = buf.Write([]byte(logEntry.Output))
			logEntry.Output = buf.String()
		}
	}

	// Create a Health object with the health state and logs
	health := &Health{
		Status:        state.Status,
		FailingStreak: state.FailingStreak,
		Log:           logs,
	}

	return health, nil
}

// writeHealthStateToLabels writes the health state to container labels
func writeHealthStateToLabels(ctx context.Context, container containerd.Container, healthState *HealthState) error {
	hs, err := healthState.ToJSONString()
	if err != nil {
		return fmt.Errorf("failed to marshal health healthState: %w", err)
	}

	lbs, err := container.Labels(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container labels: %w", err)
	}

	// Update healthState label
	lbs[labels.HealthState] = hs
	_, err = container.SetLabels(ctx, lbs)
	if err != nil {
		return fmt.Errorf("failed to update container labels: %w", err)
	}

	return nil
}

// readHealthStateFromLabels reads the health state from container labels
func readHealthStateFromLabels(ctx context.Context, container containerd.Container) (*HealthState, error) {
	lbs, err := container.Labels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container labels: %w", err)
	}

	// Check if health state label exists
	stateJSON, ok := lbs[labels.HealthState]
	if !ok {
		return nil, nil
	}

	// HealthCheckFromJSON health state from JSON
	state, err := HealthStateFromJSON(stateJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse health state: %w", err)
	}

	return state, nil
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
		return filesystem.WriteFile(healthLogPath, []byte{}, 0600)
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
