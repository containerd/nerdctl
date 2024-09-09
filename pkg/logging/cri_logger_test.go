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

/*
	Forked from https://github.com/kubernetes/kubernetes/blob/a66aad2d80dacc70025f95a8f97d2549ebd3208c/pkg/kubelet/kuberuntime/logs/logs_test.go
	Copyright The Kubernetes Authors.
	Licensed under the Apache License, Version 2.0
*/

package logging

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestReadLogs(t *testing.T) {
	file, err := os.CreateTemp("", "TestFollowLogs")
	if err != nil {
		t.Fatalf("unable to create temp file")
	}
	defer os.Remove(file.Name())
	file.WriteString(`2016-10-06T00:17:09.669794202Z stdout F line1` + "\n")
	file.WriteString(`2016-10-06T00:17:10.669794202Z stdout F line2` + "\n")
	file.WriteString(`2016-10-06T00:17:11.669794202Z stdout F line3` + "\n")

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
			err = ReadLogs(&tc.logViewOptions, stdoutBuf, stderrBuf, stopChan)

			if err != nil {
				t.Fatal(err.Error())
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

func TestParseLog(t *testing.T) {
	timestamp, err := time.Parse(time.RFC3339Nano, "2016-10-20T18:39:20.57606443Z")

	if err != nil {
		t.Fatalf("Parse Time err %s", err.Error())
	}
	logmsg := &logMessage{}
	for c, test := range []struct {
		line string
		msg  *logMessage
		err  bool
	}{
		{ // CRI log format stdout
			line: "2016-10-20T18:39:20.57606443Z stdout F cri stdout test log\n",
			msg: &logMessage{
				timestamp: timestamp,
				stream:    Stdout,
				log:       []byte("cri stdout test log\n"),
			},
		},
		{ // CRI log format stderr
			line: "2016-10-20T18:39:20.57606443Z stderr F cri stderr test log\n",
			msg: &logMessage{
				timestamp: timestamp,
				stream:    Stderr,
				log:       []byte("cri stderr test log\n"),
			},
		},
		{ // Unsupported Log format
			line: "unsupported log format test log\n",
			msg:  &logMessage{},
			err:  true,
		},
		{ // Partial CRI log line
			line: "2016-10-20T18:39:20.57606443Z stdout P cri stdout partial test log\n",
			msg: &logMessage{
				timestamp: timestamp,
				stream:    Stdout,
				log:       []byte("cri stdout partial test log"),
			},
		},
		{ // Partial CRI log line with multiple log tags.
			line: "2016-10-20T18:39:20.57606443Z stdout P:TAG1:TAG2 cri stdout partial test log\n",
			msg: &logMessage{
				timestamp: timestamp,
				stream:    Stdout,
				log:       []byte("cri stdout partial test log"),
			},
		},
	} {
		t.Logf("TestCase #%d: %+v", c, test)

		err = ParseCRILog([]byte(test.line), logmsg)
		if err != nil {
			if test.err {
				continue
			}
			t.Errorf("ParseCRILog err %s ", err.Error())
		}

		if !reflect.DeepEqual(test.msg, logmsg) {
			t.Errorf("ParseCRILog failed, msg is %#v,test.msg is %#v", logmsg, test.msg)
		}

	}
}

func TestReadLogsLimitsWithTimestamps(t *testing.T) {
	logLineFmt := "2022-10-29T16:10:22.592603036-05:00 stdout P %v\n"
	logLineNewLine := "2022-10-29T16:10:22.592603036-05:00 stdout F \n"

	tmpfile, err := os.CreateTemp("", "log.*.txt")
	if err != nil {
		t.Fatalf("unable to create temp file")
	}

	stopChan := make(chan os.Signal)

	count := 10000

	for i := 0; i < count; i++ {
		tmpfile.WriteString(fmt.Sprintf(logLineFmt, i))
	}
	tmpfile.WriteString(logLineNewLine)

	for i := 0; i < count; i++ {
		tmpfile.WriteString(fmt.Sprintf(logLineFmt, i))
	}
	tmpfile.WriteString(logLineNewLine)

	// two lines are in the buffer

	defer os.Remove(tmpfile.Name()) // clean up

	tmpfile.Close()

	var buf bytes.Buffer
	w := io.MultiWriter(&buf)

	err = ReadLogs(&LogViewOptions{LogPath: tmpfile.Name(), Tail: 0, Timestamps: true}, w, w, stopChan)
	if err != nil {
		t.Errorf("ReadLogs file %s failed %s", tmpfile.Name(), err.Error())
	}

	lineCount := 0
	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	for scanner.Scan() {
		lineCount++

		// Split the line
		ts, logline, _ := bytes.Cut(scanner.Bytes(), []byte(" "))

		// Verification
		//   1. The timestamp should exist
		//   2. The last item in the log should be 9999
		_, err = time.Parse(time.RFC3339, string(ts))
		if err != nil {
			t.Errorf("timestamp not found, err: %s", err.Error())
		}

		if !bytes.HasSuffix(logline, []byte("9999")) {
			t.Errorf("the complete log found, err: %s", err.Error())
		}
	}

	if lineCount != 2 {
		t.Errorf("should have two lines, lineCount= %d", lineCount)
	}
}
