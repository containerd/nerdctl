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
	"errors"
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

// TestLoggingProcessAdapterTrailingChunk verifies that the logger forwards all
// of the container's output, including a final chunk that has no trailing
// newline, rather than holding that chunk back until something closes the
// stream. The container's stdio FIFOs are modelled with os.Pipe; closing the
// write end models the container exiting and containerd closing the FIFO.
// Regression test for https://github.com/containerd/nerdctl/issues/5006

// TestLoggingProcessAdapterWaitError verifies that the logger does not treat a
// Wait failure as a container exit. containerd's client delivers Wait RPC
// errors through the exit channel as a synthetic ExitStatus carrying an error;
// if the logger cancelled its readers on such a delivery, all logging would
// silently stop while the container keeps running — and, in the foreground
// attach path, wedge `nerdctl run` behind the no-longer-drained logger pipes.
// The logger must instead re-arm the wait and keep reading until a real exit
// arrives. Regression test for
// https://github.com/containerd/nerdctl/issues/5137
func TestLoggingProcessAdapterWaitError(t *testing.T) {
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutR.Close()
	defer stderrR.Close()
	defer stderrW.Close()

	driver := &SyncMockDriver{}
	config := &logging.Config{
		Stdout: stdoutR,
		Stderr: stderrR,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The first wait channel delivers a Wait RPC error (a synthetic exit); the
	// second delivers a real exit once the test has verified that logging
	// survived the first delivery.
	rearmed := make(chan struct{})
	realExitCh := make(chan containerd.ExitStatus, 1)
	var waitCalls int
	var getContainerWaitMock ContainerWaitFunc = func(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error) {
		waitCalls++
		if waitCalls == 1 {
			errChan := make(chan containerd.ExitStatus, 1)
			errChan <- *containerd.NewExitStatus(255, time.Time{}, errors.New("transient wait RPC failure"))
			return errChan, nil
		}
		close(rearmed)
		return realExitCh, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- loggingProcessAdapter(ctx, driver, "testDataStore", "", getContainerWaitMock, config)
	}()

	if _, err := stdoutW.Write([]byte("before wait error\n")); err != nil {
		t.Fatal(err)
	}

	// The logger must re-arm the wait rather than cancel its readers.
	select {
	case <-rearmed:
	case <-time.After(30 * time.Second):
		t.Fatal("logger did not re-arm the container wait after the wait channel delivered an error")
	}

	// Output produced after the errored delivery must still be logged.
	if _, err := stdoutW.Write([]byte("after wait error\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		driver.mu.Lock()
		got := strings.Join(driver.receivedStdout, "")
		driver.mu.Unlock()
		if strings.Contains(got, "after wait error") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("output written after the errored wait delivery was never logged; got stdout: %q", got)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A real exit must still terminate the logger. Close both write ends so
	// the stream readers finish via EOF: on Windows, cancelreader cannot
	// cancel a blocked pipe read, so the readers must not be left waiting on
	// an open pipe when the exit is delivered.
	stdoutW.Close()
	stderrW.Close()
	realExitCh <- containerd.ExitStatus{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("logger did not terminate on the real container exit")
	}

	driver.mu.Lock()
	defer driver.mu.Unlock()
	stdout := strings.Join(driver.receivedStdout, "")
	if !strings.Contains(stdout, "before wait error") || !strings.Contains(stdout, "after wait error") {
		t.Fatalf("expected stdout to contain output from before and after the errored wait delivery, got: %q", stdout)
	}
}

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
