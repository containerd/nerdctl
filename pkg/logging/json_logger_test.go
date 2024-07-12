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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadRotatedJSONLog(t *testing.T) {
	tmpDir := t.TempDir()
	file, err := os.CreateTemp(tmpDir, "logfile")
	if err != nil {
		t.Errorf("unable to create temp file, error: %s", err.Error())
	}
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	containerStopped := make(chan os.Signal)
	// Start to follow the container's log.
	fileName := file.Name()
	go func() {
		lvOpts := LogViewOptions{
			Follow:  true,
			LogPath: fileName,
		}
		viewLogsJSONFileDirect(lvOpts, file.Name(), stdoutBuf, stderrBuf, containerStopped)
	}()

	// log in stdout
	expectedStdout := "line0\nline1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\n"
	dir := filepath.Dir(file.Name())
	baseName := filepath.Base(file.Name())

	// Write 10 lines to log file.
	// Let ReadLogs start.
	time.Sleep(50 * time.Millisecond)

	type logContent struct {
		Log    string `json:"log"`
		Stream string `json:"stream"`
		Time   string `json:"time"`
	}

	for line := 0; line < 10; line++ {
		// Write the first three lines to log file
		log := logContent{}
		log.Log = fmt.Sprintf("line%d\n", line)
		log.Stream = "stdout"
		log.Time = time.Now().Format(time.RFC3339Nano)
		time.Sleep(1 * time.Millisecond)
		logData, _ := json.Marshal(log)
		file.Write(logData)

		if line == 5 {
			file.Close()
			// Pretend to rotate the log.
			rotatedName := fmt.Sprintf("%s.%s", baseName, time.Now().Format("20060102-150405"))
			rotatedName = filepath.Join(dir, rotatedName)
			if err := os.Rename(filepath.Join(dir, baseName), rotatedName); err != nil {
				t.Errorf("failed to rotate log %q to %q, error: %s", file.Name(), rotatedName, err.Error())
				return
			}

			time.Sleep(20 * time.Millisecond)
			newF := filepath.Join(dir, baseName)
			if file, err = os.Create(newF); err != nil {
				t.Errorf("unable to create new log file, error: %s", err.Error())
				return
			}
		}
	}

	// Finished writing into the file, close it, so we can delete it later.
	err = file.Close()
	if err != nil {
		t.Errorf("could not close file, error: %s", err.Error())
	}

	time.Sleep(2 * time.Second)
	// Make the function ReadLogs end.
	close(containerStopped)

	if expectedStdout != stdoutBuf.String() {
		t.Errorf("expected: %s, acoutal: %s", expectedStdout, stdoutBuf.String())
	}
}

func TestReadJSONLogs(t *testing.T) {
	file, err := os.CreateTemp("", "TestFollowLogs")
	if err != nil {
		t.Fatalf("unable to create temp file")
	}
	defer os.Remove(file.Name())
	file.WriteString(`{"log":"line1\n","stream":"stdout","time":"2024-07-12T03:09:24.916296732Z"}` + "\n")
	file.WriteString(`{"log":"line2\n","stream":"stdout","time":"2024-07-12T03:09:24.916296732Z"}` + "\n")
	file.WriteString(`{"log":"line3\n","stream":"stdout","time":"2024-07-12T03:09:24.916296732Z"}` + "\n")

	stopChan := make(chan os.Signal)
	testCases := []struct {
		name           string
		logViewOptions LogViewOptions
		expected       string
	}{
		{
			name: "default log options should output all lines",
			logViewOptions: LogViewOptions{
				LogPath: file.Name(),
				Tail:    0,
			},
			expected: "line1\nline2\nline3\n",
		},
		{
			name: "using Tail 2 should output last 2 lines",
			logViewOptions: LogViewOptions{
				LogPath: file.Name(),
				Tail:    2,
			},
			expected: "line2\nline3\n",
		},
		{
			name: "using Tail 4 should output all lines when the log has less than 4 lines",
			logViewOptions: LogViewOptions{
				LogPath: file.Name(),
				Tail:    4,
			},
			expected: "line1\nline2\nline3\n",
		},
		{
			name: "using Tail 0 should output all",
			logViewOptions: LogViewOptions{
				LogPath: file.Name(),
				Tail:    0,
			},
			expected: "line1\nline2\nline3\n",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdoutBuf := bytes.NewBuffer(nil)
			stderrBuf := bytes.NewBuffer(nil)
			err = viewLogsJSONFileDirect(tc.logViewOptions, file.Name(), stdoutBuf, stderrBuf, stopChan)

			if err != nil {
				t.Fatalf(err.Error())
			}
			if stderrBuf.Len() > 0 {
				t.Fatalf("Stderr: %v", stderrBuf.String())
			}
			if actual := stdoutBuf.String(); tc.expected != actual {
				t.Fatalf("Actual output does not match expected.\nActual:  %v\nExpected: %v\n", actual, tc.expected)
			}
		})
	}
}
