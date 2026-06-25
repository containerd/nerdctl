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
	"bufio"
	"bytes"
	"context"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
)

type MockDriver struct {
	processed      bool
	receivedStdout []string
	receivedStderr []string
}

func (m *MockDriver) Init(dataStore, ns, id string) error {
	return nil
}

func (m *MockDriver) PreProcess(ctx context.Context, dataStore string, config *logging.Config) error {
	return nil
}

func (m *MockDriver) Process(stdout <-chan string, stderr <-chan string) error {
	for line := range stdout {
		m.receivedStdout = append(m.receivedStdout, line)
	}
	for line := range stderr {
		m.receivedStderr = append(m.receivedStderr, line)
	}
	m.processed = true
	return nil
}

func (m *MockDriver) PostProcess() error {
	return nil
}

// SyncMockDriver implements SyncDriver, recording the entries written to it.
type SyncMockDriver struct {
	mu             sync.Mutex
	receivedStdout []string
	receivedStderr []string
}

func (m *SyncMockDriver) Init(dataStore, ns, id string) error { return nil }
func (m *SyncMockDriver) PreProcess(ctx context.Context, dataStore string, config *logging.Config) error {
	return nil
}
func (m *SyncMockDriver) Process(stdout <-chan string, stderr <-chan string) error {
	// Not used on the synchronous path (the logger calls WriteLogEntry instead),
	// but must satisfy the Driver interface.
	return nil
}
func (m *SyncMockDriver) PostProcess() error { return nil }
func (m *SyncMockDriver) WriteLogEntry(stream, line string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if stream == streamStdout {
		m.receivedStdout = append(m.receivedStdout, line)
	} else {
		m.receivedStderr = append(m.receivedStderr, line)
	}
	return nil
}

func TestLoggingProcessAdapter(t *testing.T) {
	// Will process a normal String to stdout and a bigger one to stderr
	normalString := generateRandomString(1024)

	// Generate 64KB of random text of bufio MaxScanTokenSize
	// https://github.com/containerd/nerdctl/issues/3343
	hugeString := generateRandomString(bufio.MaxScanTokenSize)

	// Prepare mock driver and logging config
	driver := &MockDriver{}
	stdoutBuffer := bytes.NewBufferString(normalString)
	stderrBuffer := bytes.NewBufferString(hugeString)
	config := &logging.Config{
		Stdout: stdoutBuffer,
		Stderr: stderrBuffer,
	}

	// Execute the logging process adapter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var getContainerWaitMock ContainerWaitFunc = func(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error) {
		exitChan := make(chan containerd.ExitStatus, 1)
		time.Sleep(50 * time.Millisecond)
		exitChan <- containerd.ExitStatus{}
		return exitChan, nil
	}

	err := loggingProcessAdapter(ctx, driver, "testDataStore", "", getContainerWaitMock, config)
	if err != nil {
		t.Fatal(err)
	}

	// let bufio read the buffer
	time.Sleep(50 * time.Millisecond)

	// Verify that the driver methods were called
	if !driver.processed {
		t.Fatal("process should be processed")
	}

	// Verify that the driver received the expected data
	stdout := strings.Join(driver.receivedStdout, "\n")
	stderr := strings.Join(driver.receivedStderr, "\n")

	if stdout != normalString {
		t.Fatalf("stdout is %s, expected %s", stdout, normalString)
	}

	if stderr != hugeString {
		t.Fatalf("stderr is %s, expected %s", stderr, hugeString)
	}
}

// TestLoggingProcessAdapterTrailingChunk verifies that the logger forwards all
// of the container's output, including a final chunk that has no trailing
// newline, rather than holding that chunk back until something closes the
// stream. The container's stdio FIFOs are modelled with os.Pipe; closing the
// write end models the container exiting and containerd closing the FIFO.
// Regression test for https://github.com/containerd/nerdctl/issues/5006
func TestLoggingProcessAdapterTrailingChunk(t *testing.T) {
	const expected = "'Hello World!\nThere is no newline'"

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutR.Close()
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stderrR.Close()

	driver := &MockDriver{}
	config := &logging.Config{
		Stdout: stdoutR,
		Stderr: stderrR,
	}

	// Write the container's output, including a trailing chunk without a newline,
	// then close the write ends to model the container exiting.
	if _, err := stdoutW.WriteString(expected); err != nil {
		t.Fatal(err)
	}
	stdoutW.Close()
	stderrW.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// getContainerWait never reports an exit here: completion is driven by the
	// FIFOs reaching EOF, as it usually is in practice.
	var getContainerWaitMock ContainerWaitFunc = func(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error) {
		return make(chan containerd.ExitStatus), nil
	}

	if err := loggingProcessAdapter(ctx, driver, "testDataStore", "", getContainerWaitMock, config); err != nil {
		t.Fatal(err)
	}

	if actual := strings.Join(driver.receivedStdout, ""); actual != expected {
		t.Fatalf("stdout is %q, expected %q", actual, expected)
	}
}

// TestLoggingProcessAdapterSyncTrailingChunk verifies the same trailing-chunk
// behaviour for a driver that writes synchronously (SyncDriver), which is the
// path that protects the final chunk from the container's abrupt teardown.
// Regression test for https://github.com/containerd/nerdctl/issues/5006
func TestLoggingProcessAdapterSyncTrailingChunk(t *testing.T) {
	const expected = "'Hello World!\nThere is no newline'"

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutR.Close()
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stderrR.Close()

	driver := &SyncMockDriver{}
	config := &logging.Config{
		Stdout: stdoutR,
		Stderr: stderrR,
	}

	if _, err := stdoutW.WriteString(expected); err != nil {
		t.Fatal(err)
	}
	stdoutW.Close()
	stderrW.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var getContainerWaitMock ContainerWaitFunc = func(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error) {
		return make(chan containerd.ExitStatus), nil
	}

	if err := loggingProcessAdapter(ctx, driver, "testDataStore", "", getContainerWaitMock, config); err != nil {
		t.Fatal(err)
	}

	if actual := strings.Join(driver.receivedStdout, ""); actual != expected {
		t.Fatalf("stdout is %q, expected %q", actual, expected)
	}
}

// generateRandomString creates a random string of the given size.
func generateRandomString(size int) string {
	characters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var sb strings.Builder
	for i := 0; i < size; i++ {
		sb.WriteByte(characters[rand.Intn(len(characters))])
	}
	return sb.String()
}
