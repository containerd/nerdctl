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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/logging/jsonfile"
	"github.com/containerd/nerdctl/v2/pkg/logging/tail"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/docker/go-units"
	"github.com/fahedouch/go-logrotate"
	"github.com/fsnotify/fsnotify"
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
			log.L.Warnf("log-opt %s is ignored for json-file log driver", key)
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
		// FIXME: this is a workaround for the actual issue, not a real solution
		// https://github.com/containerd/nerdctl/issues/3187
		if errors.Is(err, errdefs.ErrNotFound) {
			log.L.Warnf("Racing log file creation. Pausing briefly.")
			time.Sleep(200 * time.Millisecond)
			_, err = os.Stat(logFilePath)
		}
		if err != nil {
			return fmt.Errorf("failed to stat JSON log file %w", err)
		}
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
	defer func() { fin.Close() }()

	// Search start point based on tail line.
	start, err := tail.FindTailLineStartIndex(fin, lvopts.Tail)
	if err != nil {
		return fmt.Errorf("failed to tail %d lines of JSON logfile %q: %w", lvopts.Tail, jsonLogFilePath, err)
	}

	if _, err := fin.Seek(start, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek in log file %q from %d position: %w", jsonLogFilePath, start, err)
	}

	limitedMode := (lvopts.Tail > 0) && (!lvopts.Follow)
	limitedNum := lvopts.Tail
	var stop bool
	var watcher *fsnotify.Watcher
	baseName := filepath.Base(jsonLogFilePath)
	dir := filepath.Dir(jsonLogFilePath)
	retryTimes := 2
	backBytes := 0

	for {
		select {
		case <-stopChannel:
			log.L.Debug("received stop signal while re-reading JSON logfile, returning")
			return nil
		default:
			if stop || (limitedMode && limitedNum == 0) {
				log.L.Debugf("finished parsing log JSON filefile, path: %s", jsonLogFilePath)
				return nil
			}

			if line, err := jsonfile.Decode(stdout, stderr, fin, lvopts.Timestamps, lvopts.Since, lvopts.Until); err != nil {
				if len(line) > 0 {
					time.Sleep(5 * time.Millisecond)
					if retryTimes == 0 {
						log.L.Infof("finished parsing log JSON filefile, path: %s, line: %s", jsonLogFilePath, string(line))
						return fmt.Errorf("error occurred while doing read of JSON logfile %q: %s, retryTimes: %d", jsonLogFilePath, err, retryTimes)
					}
					retryTimes--
					backBytes = len(line)
				} else {
					return fmt.Errorf("error occurred while doing read of JSON logfile %q: %s", jsonLogFilePath, err)
				}
			} else {
				retryTimes = 2
				backBytes = 0
			}

			if lvopts.Follow {
				// Get the current file handler's seek.
				lastPos, err := fin.Seek(int64(-backBytes), io.SeekCurrent)
				if err != nil {
					return fmt.Errorf("error occurred while trying to seek JSON logfile %q at position %d: %s", jsonLogFilePath, lastPos, err)
				}

				if watcher == nil {
					// Initialize the watcher if it has not been initialized yet.
					if watcher, err = NewLogFileWatcher(dir); err != nil {
						return err
					}
					defer watcher.Close()
					// If we just created the watcher, try again to read as we might have missed
					// the event.
					continue
				}

				var recreated bool
				// Wait until the next log change.
				recreated, err = startTail(context.Background(), baseName, watcher)
				if err != nil {
					return err
				}
				if recreated {
					newF, err := openFileShareDelete(jsonLogFilePath)
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							//If the user application outputs logs too quickly,
							//There is a slight possibility that nerdctl has just rotated the log file,
							//try opening it once more.
							time.Sleep(10 * time.Millisecond)
						}
						newF, err = openFileShareDelete(jsonLogFilePath)
						if err != nil {
							return fmt.Errorf("failed to open JSON logfile %q: %w", jsonLogFilePath, err)
						}
					}
					fin.Close()
					fin = newF
				}
				continue
			}
			stop = true
			// Give the OS a second to breathe before re-opening the file:
		}
	}
}
