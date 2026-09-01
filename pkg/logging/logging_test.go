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
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

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

	err := loggingProcessAdapter(ctx, driver, "testDataStore", "", getContainerWaitMock, config, nil, nil)
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

	if err := loggingProcessAdapter(ctx, driver, "testDataStore", "", getContainerWaitMock, config, nil, nil); err != nil {
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

	if err := loggingProcessAdapter(ctx, driver, "testDataStore", "", getContainerWaitMock, config, nil, nil); err != nil {
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

func TestLoggingProcessAdapterTeesRawOutput(t *testing.T) {
	// The broker has to see the container's bytes exactly as they arrive,
	// before they are split into log lines, so that terminal escape sequences
	// and partial lines reach an attached session unchanged.
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	config := &logging.Config{
		ID:        "test-container",
		Namespace: "test-namespace",
		Stdout:    stdoutR,
		Stderr:    stderrR,
	}

	var mu sync.Mutex
	teed := map[string][]byte{}
	tee := func(stream string, p []byte) {
		mu.Lock()
		defer mu.Unlock()
		teed[stream] = append(teed[stream], p...)
	}

	driver := &MockDriver{}
	exitCh := make(chan containerd.ExitStatus, 1)
	wait := func(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error) {
		return exitCh, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- loggingProcessAdapter(context.Background(), driver, t.TempDir(), "", wait, config, tee, nil)
	}()

	// A chunk with no trailing newline is exactly the case the log driver
	// buffers but an attached terminal must see immediately.
	_, err := stdoutW.Write([]byte("prompt$ "))
	assert.NilError(t, err)
	_, err = stderrW.Write([]byte("warning\n"))
	assert.NilError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := string(teed["stdout"]) == "prompt$ " && string(teed["stderr"]) == "warning\n"
		mu.Unlock()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	gotOut, gotErr := string(teed["stdout"]), string(teed["stderr"])
	mu.Unlock()
	assert.Equal(t, gotOut, "prompt$ ")
	assert.Equal(t, gotErr, "warning\n")

	stdoutW.Close()
	stderrW.Close()
	exitCh <- *containerd.NewExitStatus(0, time.Now(), nil)

	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("loggingProcessAdapter did not return")
	}
}

// slowDriver lingers in Process after its channels are closed, the way a
// network-backed log driver does when it is still flushing.
type slowDriver struct {
	MockDriver
	release chan struct{}
}

func (d *slowDriver) Process(stdout <-chan string, stderr <-chan string) error {
	// Drain the way MockDriver does, then linger the way a network-backed
	// driver does while it flushes.
	if err := d.MockDriver.Process(stdout, stderr); err != nil {
		return err
	}
	<-d.release
	return nil
}

func TestLoggingProcessAdapterStopsTheBrokerBeforeTheDriverFinishes(t *testing.T) {
	// The attached sessions must be told the container is gone as soon as its
	// output has been read. Waiting for the log driver first would let a session
	// time out and report a failure for a container that ran fine.
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	config := &logging.Config{
		ID:        "test-container",
		Namespace: "test-namespace",
		Stdout:    stdoutR,
		Stderr:    stderrR,
	}

	driver := &slowDriver{release: make(chan struct{})}
	exitCh := make(chan containerd.ExitStatus, 1)
	wait := func(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error) {
		return exitCh, nil
	}

	stopped := make(chan bool, 1)
	done := make(chan error, 1)
	go func() {
		done <- loggingProcessAdapter(context.Background(), driver, t.TempDir(), "", wait, config, nil,
			func(exited bool) { stopped <- exited })
	}()

	stdoutW.Close()
	stderrW.Close()
	exitCh <- *containerd.NewExitStatus(0, time.Now(), nil)

	select {
	case exited := <-stopped:
		// containerd reported a clean exit above, so the sessions may be told.
		assert.Equal(t, exited, true)
	case <-time.After(5 * time.Second):
		t.Fatal("the broker was not stopped while the driver was still running")
	}

	close(driver.release)
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("loggingProcessAdapter did not return")
	}
}
