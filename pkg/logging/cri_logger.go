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
	Forked from https://github.com/kubernetes/kubernetes/blob/a66aad2d80dacc70025f95a8f97d2549ebd3208c/pkg/kubelet/kuberuntime/logs/logs.go
	Copyright The Kubernetes Authors.
	Licensed under the Apache License, Version 2.0
*/

package logging

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/logging/tail"
)

// LogStreamType is the type of the stream in CRI container log.
type LogStreamType string

const (
	// Stdout is the stream type for stdout.
	Stdout LogStreamType = "stdout"
	// Stderr is the stream type for stderr.
	Stderr LogStreamType = "stderr"
)

// LogTag is the tag of a log line in CRI container log.
// Currently defined log tags:
// * First tag: Partial/Full - P/F.
// The field in the container log format can be extended to include multiple
// tags by using a delimiter, but changes should be rare. If it becomes clear
// that better extensibility is desired, a more extensible format (e.g., json)
// should be adopted as a replacement and/or addition.
type LogTag string

const (
	// LogTagPartial means the line is part of multiple lines.
	LogTagPartial LogTag = "P"
	// LogTagFull means the line is a single full line or the end of multiple lines.
	LogTagFull LogTag = "F"
	// LogTagDelimiter is the delimiter for different log tags.
	LogTagDelimiter = ":"
)

// Loads log entries from logfiles produced by the Text-logger driver and forwards
// them to the provided io.Writers after applying the provided logging options.
func viewLogsCRI(lvopts LogViewOptions, stdout, stderr io.Writer, stopChannel chan os.Signal) error {
	if lvopts.LogPath == "" {
		return fmt.Errorf("logpath is nil ")
	}

	return ReadLogs(&lvopts, stdout, stderr, stopChannel)
}

// ReadLogs read the container log and redirect into stdout and stderr.
// Note that containerID is only needed when following the log, or else
// just pass in empty string "".
func ReadLogs(opts *LogViewOptions, stdout, stderr io.Writer, stopChannel chan os.Signal) error {
	var logPath = opts.LogPath
	evaluated, err := filepath.EvalSymlinks(logPath)
	if err != nil {
		return fmt.Errorf("failed to try resolving symlinks in path %q: %v", logPath, err)
	}
	logPath = evaluated
	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file %q: %v", logPath, err)
	}
	defer f.Close()

	// Search start point based on tail line.
	start, err := tail.FindTailLineStartIndex(f, opts.Tail)
	if err != nil {
		return fmt.Errorf("failed to tail %d lines of log file %q: %v", opts.Tail, logPath, err)
	}

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek in log file %q: %v", logPath, err)
	}

	limitedMode := (opts.Tail > 0) && (!opts.Follow)
	limitedNum := opts.Tail
	// Start parsing the logs.
	r := bufio.NewReader(f)

	var stop bool
	isNewLine := true
	writer := newLogWriter(stdout, stderr, opts)
	msg := &logMessage{}
	for {
		select {
		case <-stopChannel:
			log.L.Debugf("received stop signal while reading cri logfile, returning")
			return nil
		default:
			if stop || (limitedMode && limitedNum == 0) {
				log.L.Debugf("finished parsing log file, path: %s", logPath)
				return nil
			}
			l, err := r.ReadBytes(eol[0])
			if err != nil {
				if err != io.EOF { // This is an real error
					return fmt.Errorf("failed to read log file %q: %v", logPath, err)
				}
				if opts.Follow {

					// Reset seek so that if this is an incomplete line,
					// it will be read again.
					if _, err := f.Seek(-int64(len(l)), io.SeekCurrent); err != nil {
						return fmt.Errorf("failed to reset seek in log file %q: %v", logPath, err)
					}

					// If the container exited consume data until the next EOF
					continue
				}
				// Should stop after writing the remaining content.
				stop = true
				if len(l) == 0 {
					continue
				}
				log.L.Debugf("incomplete line in log file, path: %s, line: %s", logPath, l)
			}

			// Parse the log line.
			msg.reset()
			if err := ParseCRILog(l, msg); err != nil {
				log.L.WithError(err).Errorf("failed when parsing line in log file, path: %s, line: %s", logPath, l)
				continue
			}
			// Write the log line into the stream.
			if err := writer.write(msg, isNewLine); err != nil {
				if err == errMaximumWrite {
					log.L.Debugf("finished parsing log file, hit bytes limit path: %s", logPath)
					return nil
				}
				log.L.WithError(err).Errorf("failed when writing line to log file, path: %s, line: %s", logPath, l)
				return err
			}
			if limitedMode {
				limitedNum--
			}
			if len(msg.log) > 0 {
				isNewLine = msg.log[len(msg.log)-1] == eol[0]
			} else {
				isNewLine = true
			}
		}
	}
}

var (
	// eol is the end-of-line sign in the log.
	eol = []byte{'\n'}
	// delimiter is the delimiter for timestamp and stream type in log line.
	delimiter = []byte{' '}
	// tagDelimiter is the delimiter for log tags.
	tagDelimiter = []byte(":")
)

// logWriter controls the writing into the stream based on the log options.
type logWriter struct {
	stdout io.Writer
	stderr io.Writer
	opts   *LogViewOptions
	remain int64
}

// errMaximumWrite is returned when all bytes have been written.
var errMaximumWrite = errors.New("maximum write")

// errShortWrite is returned when the message is not fully written.
var errShortWrite = errors.New("short write")

func newLogWriter(stdout io.Writer, stderr io.Writer, opts *LogViewOptions) *logWriter {
	w := &logWriter{
		stdout: stdout,
		stderr: stderr,
		opts:   opts,
		remain: math.MaxInt64, // initialize it as infinity
	}
	//if opts.bytes >= 0 {
	//	w.remain = opts.bytes
	//}
	return w
}

// writeLogs writes logs into stdout, stderr.
func (w *logWriter) write(msg *logMessage, addPrefix bool) error {

	//if msg.timestamp.Before(ts) {
	//	// Skip the line because it's older than since
	//	return nil
	//}
	line := msg.log
	if w.opts.Timestamps && addPrefix {
		prefix := append([]byte(msg.timestamp.Format(log.RFC3339NanoFixed)), delimiter[0])
		line = append(prefix, line...)
	}
	// If the line is longer than the remaining bytes, cut it.
	if int64(len(line)) > w.remain {
		line = line[:w.remain]
	}
	// Get the proper stream to write to.
	var stream io.Writer
	switch msg.stream {
	case Stdout:
		stream = w.stdout
	case Stderr:
		stream = w.stderr
	default:
		return fmt.Errorf("unexpected stream type %q", msg.stream)
	}
	n, err := stream.Write(line)
	w.remain -= int64(n)
	if err != nil {
		return err
	}
	// If the line has not been fully written, return errShortWrite
	if n < len(line) {
		return errShortWrite
	}
	// If there are no more bytes left, return errMaximumWrite
	if w.remain <= 0 {
		return errMaximumWrite
	}
	return nil
}

// logMessage is the CRI internal log type.
type logMessage struct {
	timestamp time.Time
	stream    LogStreamType
	log       []byte
}

// reset the log to nil.
func (l *logMessage) reset() {
	l.timestamp = time.Time{}
	l.stream = ""
	l.log = nil
}

// ParseCRILog parses logs in CRI log format. CRI Log format example:
//
//	2016-10-06T00:17:09.669794202Z stdout P log content 1
//	2016-10-06T00:17:09.669794203Z stderr F log content 2
func ParseCRILog(log []byte, msg *logMessage) error {
	var err error
	// Parse timestamp
	idx := bytes.Index(log, delimiter)
	if idx < 0 {
		return fmt.Errorf("timestamp is not found")
	}
	msg.timestamp, err = time.Parse(time.RFC3339Nano, string(log[:idx]))
	if err != nil {
		return fmt.Errorf("unexpected timestamp format %q: %v", time.RFC3339Nano, err)
	}

	// Parse stream type
	log = log[idx+1:]
	idx = bytes.Index(log, delimiter)
	if idx < 0 {
		return fmt.Errorf("stream type is not found")
	}
	msg.stream = LogStreamType(log[:idx])
	if msg.stream != Stdout && msg.stream != Stderr {
		return fmt.Errorf("unexpected stream type %q", msg.stream)
	}

	// Parse log tag
	log = log[idx+1:]
	idx = bytes.Index(log, delimiter)
	if idx < 0 {
		return fmt.Errorf("log tag is not found")
	}
	// Keep this forward compatible.
	tags := bytes.Split(log[:idx], tagDelimiter)
	partial := LogTag(tags[0]) == LogTagPartial
	// Trim the tailing new line if this is a partial line.
	if partial && len(log) > 0 && log[len(log)-1] == '\n' {
		log = log[:len(log)-1]
	}

	// Get log content
	msg.log = log[idx+1:]

	return nil
}
