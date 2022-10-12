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
	"os"
	"path/filepath"
	"strconv"

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
	Opts map[string]string
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
	// TODO: move this logic to pkg/logging
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

func (jsonLogger *JSONLogger) Process(dataStore string, config *logging.Config) error {
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
	return jsonfile.Encode(l, config.Stdout, config.Stderr)
}
