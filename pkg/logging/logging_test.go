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
	"strings"
	"testing"
	"time"

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

func (m *MockDriver) PreProcess(dataStore string, config *logging.Config) error {
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

	err := loggingProcessAdapter(ctx, driver, "testDataStore", config)
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

// generateRandomString creates a random string of the given size.
func generateRandomString(size int) string {
	characters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var sb strings.Builder
	for i := 0; i < size; i++ {
		sb.WriteByte(characters[rand.Intn(len(characters))])
	}
	return sb.String()
}
