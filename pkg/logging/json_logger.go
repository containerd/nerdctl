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
	"os"
	"path/filepath"
	"strconv"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/nerdctl/pkg/logging/jsonfile"
	"github.com/docker/go-units"
	"github.com/natefinch/lumberjack"
)

type JsonLogger struct {
	capVal  int64
	maxFile int
}

func (jsonLogger *JsonLogger) init(argsMap map[string]string) error {
	if argsMap[MagicArgv1] == "" {
		return errors.New("got empty data store")
	}
	var capVal int64
	capVal = -1
	if capacity, ok := argsMap[MaxSize]; ok {
		var err error
		capVal, err = units.FromHumanSize(capacity)
		if err != nil {
			return err
		}
		if capVal <= 0 {
			return fmt.Errorf("max-size must be a positive number")
		}
	}
	jsonLogger.capVal = capVal
	maxFile := 1
	if maxFileString, ok := argsMap[MaxFile]; ok {
		var err error
		maxFile, err = strconv.Atoi(maxFileString)
		if err != nil {
			return err
		}
		if maxFile < 1 {
			return fmt.Errorf("max-file cannot be less than 1")
		}
	}
	jsonLogger.maxFile = maxFile - 1
	return nil
}

func (jsonLogger *JsonLogger) getLoggerFunc(argsMap map[string]string) logging.LoggerFunc {
	return func(_ context.Context, config *logging.Config, ready func() error) error {
		if config.Namespace == "" || config.ID == "" {
			return errors.New("got invalid config")
		}
		logJSONFilePath := jsonfile.Path(argsMap[MagicArgv1], config.Namespace, config.ID)
		if err := os.MkdirAll(filepath.Dir(logJSONFilePath), 0700); err != nil {
			return err
		}
		if err := ready(); err != nil {
			return err
		}
		l := &lumberjack.Logger{
			Filename: logJSONFilePath,
		}
		//maxSize Defaults to unlimited.
		l.MaxBytes = jsonLogger.capVal
		maxFile := 1
		// MaxBackups does not include file to write logs to
		l.MaxBackups = maxFile
		return jsonfile.Encode(l, config.Stdout, config.Stderr)
	}
}
