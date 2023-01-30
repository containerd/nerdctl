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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/nerdctl/pkg/logging/jsonfile"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/go-units"
	"github.com/fahedouch/go-logrotate"
	"github.com/sirupsen/logrus"
)

var JSONDriverLogOpts = []string{
	LogPath,
	MaxSize,
	MaxFile,
}

type JSONLogger struct {
	Opts   map[string]string
	logger *logrotate.Logger
}

func JSONFileLogOptsValidate(logOptMap map[string]string) error {
	for key := range logOptMap {
		if !strutil.InStringSlice(JSONDriverLogOpts, key) {
			logrus.Warnf("log-opt %s is ignored for json-file log driver", key)
		}
	}
	return nil
}

func (jsonLogger *JSONLogger) Init(dataStore, ns, id string) error {
	// Initialize the log file (https://github.com/containerd/nerdctl/issues/1071)
	var jsonFilePath string
	if logPath, ok := jsonLogger.Opts[LogPath]; ok {
		jsonFilePath = logPath
	} else {
		jsonFilePath = jsonfile.Path(dataStore, ns, id)
	}
	if err := os.MkdirAll(filepath.Dir(jsonFilePath), 0700); err != nil {
		return err
	}
	if _, err := os.Stat(jsonFilePath); errors.Is(err, os.ErrNotExist) {
		if writeErr := os.WriteFile(jsonFilePath, []byte{}, 0600); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (jsonLogger *JSONLogger) PreProcess(dataStore string, config *logging.Config) error {
	var jsonFilePath string
	if logPath, ok := jsonLogger.Opts[LogPath]; ok {
		jsonFilePath = logPath
	} else {
		jsonFilePath = jsonfile.Path(dataStore, config.Namespace, config.ID)
	}
	l := &logrotate.Logger{
		Filename: jsonFilePath,
	}
	//maxSize Defaults to unlimited.
	var capVal int64
	capVal = -1
	if capacity, ok := jsonLogger.Opts[MaxSize]; ok {
		var err error
		capVal, err = units.FromHumanSize(capacity)
		if err != nil {
			return err
		}
		if capVal <= 0 {
			return fmt.Errorf("max-size must be a positive number")
		}
	}
	l.MaxBytes = capVal
	maxFile := 1
	if maxFileString, ok := jsonLogger.Opts[MaxFile]; ok {
		var err error
		maxFile, err = strconv.Atoi(maxFileString)
		if err != nil {
			return err
		}
		if maxFile < 1 {
			return fmt.Errorf("max-file cannot be less than 1")
		}
	}
	// MaxBackups does not include file to write logs to
	l.MaxBackups = maxFile - 1
	jsonLogger.logger = l
	return nil
}

func (jsonLogger *JSONLogger) Process(stdout <-chan string, stderr <-chan string) error {
	return jsonfile.Encode(stdout, stderr, jsonLogger.logger)
}

func (jsonLogger *JSONLogger) PostProcess() error {
	return nil
}

// Loads log entries from logfiles produced by the json-logger driver and forwards
// them to the provided io.Writers after applying the provided logging options.
func viewLogsJSONFile(lvopts LogViewOptions, stdout, stderr io.Writer, stopChannel chan os.Signal) error {
	logFilePath := jsonfile.Path(lvopts.DatastoreRootPath, lvopts.Namespace, lvopts.ContainerID)
	if _, err := os.Stat(logFilePath); err != nil {
		return fmt.Errorf("failed to stat JSON log file ")
	}

	if checkExecutableAvailableInPath("tail") {
		return viewLogsJSONFileThroughTailExec(lvopts, logFilePath, stdout, stderr, stopChannel)
	}
	return viewLogsJSONFileDirect(lvopts, logFilePath, stdout, stderr, stopChannel)
}

// Loads JSON log entries directly from the provided JSON log file.
// If `LogViewOptions.Follow` is provided, it will refresh and re-read the file until
// it receives something through the stopChannel.
func viewLogsJSONFileDirect(lvopts LogViewOptions, jsonLogFilePath string, stdout, stderr io.Writer, stopChannel chan os.Signal) error {
	fin, err := os.OpenFile(jsonLogFilePath, os.O_RDONLY, 0400)
	if err != nil {
		return err
	}
	defer fin.Close()
	err = jsonfile.Decode(stdout, stderr, fin, lvopts.Timestamps, lvopts.Since, lvopts.Until, lvopts.Tail)
	if err != nil {
		return fmt.Errorf("error occurred while doing initial read of JSON logfile %q: %s", jsonLogFilePath, err)
	}

	if lvopts.Follow {
		// Get the current file handler's seek.
		lastPos, err := fin.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("error occurred while trying to seek JSON logfile %q at position %d: %s", jsonLogFilePath, lastPos, err)
		}
		fin.Close()
		for {
			select {
			case <-stopChannel:
				logrus.Debugf("received stop signal while re-reading JSON logfile, returning")
				return nil
			default:
				// Re-open the file and seek to the last-consumed offset.
				fin, err = os.OpenFile(jsonLogFilePath, os.O_RDONLY, 0400)
				if err != nil {
					fin.Close()
					return fmt.Errorf("error occurred while trying to re-open JSON logfile %q: %s", jsonLogFilePath, err)
				}
				_, err = fin.Seek(lastPos, 0)
				if err != nil {
					fin.Close()
					return fmt.Errorf("error occurred while trying to seek JSON logfile %q at position %d: %s", jsonLogFilePath, lastPos, err)
				}

				err = jsonfile.Decode(stdout, stderr, fin, lvopts.Timestamps, lvopts.Since, lvopts.Until, 0)
				if err != nil {
					fin.Close()
					return fmt.Errorf("error occurred while doing follow-up decoding of JSON logfile %q at starting position %d: %s", jsonLogFilePath, lastPos, err)
				}

				// Record current file seek position before looping again.
				lastPos, err = fin.Seek(0, io.SeekCurrent)
				if err != nil {
					fin.Close()
					return fmt.Errorf("error occurred while trying to seek JSON logfile %q at current position: %s", jsonLogFilePath, err)
				}
				fin.Close()
			}
			// Give the OS a second to breathe before re-opening the file:
			time.Sleep(time.Second)
		}
	}
	return nil
}

// Loads logs through the `tail` executable.
func viewLogsJSONFileThroughTailExec(lvopts LogViewOptions, jsonLogFilePath string, stdout, stderr io.Writer, stopChannel chan os.Signal) error {
	var args []string

	args = append(args, "-n")
	if lvopts.Tail == 0 {
		args = append(args, "+0")
	} else {
		args = append(args, fmt.Sprintf("%d", lvopts.Tail))
	}

	if lvopts.Follow {
		args = append(args, "-f")
	}
	args = append(args, jsonLogFilePath)
	cmd := exec.Command("tail", args...)
	cmd.Stderr = os.Stderr
	r, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Setup killing goroutine:
	go func() {
		<-stopChannel
		logrus.Debugf("killing tail logs process with PID: %d", cmd.Process.Pid)
		cmd.Process.Kill()
	}()

	return jsonfile.Decode(stdout, stderr, r, lvopts.Timestamps, lvopts.Since, lvopts.Until, 0)
}
