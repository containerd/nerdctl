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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/nerdctl/pkg/logging/jsonfile"
	units "github.com/docker/go-units"
	"github.com/natefinch/lumberjack"
)

const (
	// MagicArgv1 is the magic argv1 for the containerd runtime v2 logging plugin mode.
	MagicArgv1 = "_NERDCTL_INTERNAL_LOGGING"
	MaxSize    = "max-size"
	MaxFile    = "max-file"
)

// Main is the entrypoint for the containerd runtime v2 logging plugin mode.
//
// Should be called only if argv1 == MagicArgv1.
func Main(argv2 string) error {
	fn, err := getLoggerFunc(argv2)
	if err != nil {
		return err
	}
	logging.Run(fn)
	return nil
}

// LogConfig is marshalled as "log-config.json"
type LogConfig struct {
	Drivers []LogDriverConfig `json:"drivers,omitempty"`
}

// LogDriverConfig is defined per a driver
type LogDriverConfig struct {
	Driver string            `json:"driver"`
	Opts   map[string]string `json:"opts,omitempty"`
}

// LogConfigFilePath returns the path of log-config.json
func LogConfigFilePath(dataStore, ns, id string) string {
	return filepath.Join(dataStore, "containers", ns, id, "log-config.json")
}

func getLoggerFunc(dataStore string) (logging.LoggerFunc, error) {
	if dataStore == "" {
		return nil, errors.New("got empty data store")
	}
	return func(_ context.Context, config *logging.Config, ready func() error) error {
		if config.Namespace == "" || config.ID == "" {
			return errors.New("got invalid config")
		}
		jsonFileDriverOpts := make(map[string]string)
		logConfigFilePath := LogConfigFilePath(dataStore, config.Namespace, config.ID)
		if _, err := os.Stat(logConfigFilePath); err == nil {
			var logConfig LogConfig
			logConfigFileB, err := os.ReadFile(logConfigFilePath)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(logConfigFileB, &logConfig); err != nil {
				return err
			}
			for _, f := range logConfig.Drivers {
				switch f.Driver {
				case "json-file":
					jsonFileDriverOpts = f.Opts
				default:
					return fmt.Errorf("unknown driver %q", f.Driver)
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			// the file does not exist if the container was created with nerdctl < 0.20
			return err
		}

		logJSONFilePath := jsonfile.Path(dataStore, config.Namespace, config.ID)
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
		var capVal int64
		capVal = -1
		if capacity, ok := jsonFileDriverOpts[MaxSize]; ok {
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
		if maxFileString, ok := jsonFileDriverOpts[MaxFile]; ok {
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
	}, nil
}
