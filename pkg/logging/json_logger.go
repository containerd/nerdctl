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
	Opts map[string]string
}

func (jsonLogger *JsonLogger) Process(dataStore string, config *logging.Config) error {
	logJSONFilePath := jsonfile.Path(dataStore, config.Namespace, config.ID)
	if err := os.MkdirAll(filepath.Dir(logJSONFilePath), 0700); err != nil {
		return err
	}
	l := &lumberjack.Logger{
		Filename: logJSONFilePath,
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
