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

	"github.com/containerd/containerd/runtime/v2/logging"
)

const (
	// MagicArgv1 is the magic argv1 for the containerd runtime v2 logging plugin mode.
	MagicArgv1 = "_NERDCTL_INTERNAL_LOGGING"
	MaxSize    = "max-size"
	MaxFile    = "max-file"
	Tag        = "tag"
)

type LogDriver interface {
	Process(dataStore string, config *logging.Config) error
}

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
		var driver LogDriver
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
			switch logConfig.Driver {
			case "json-file":
				driver = &JsonLogger{
					Opts: logConfig.Opts,
				}
			case "journald":
				driver = &JournaldLogger{
					Opts: logConfig.Opts,
				}
			case "fluentd":
				driver = &FluentdLogger{
					Opts: logConfig.Opts,
				}
			default:
				return fmt.Errorf("unknown driver %q", logConfig.Driver)
			}
			if err := ready(); err != nil {
				return err
			}
			return driver.Process(dataStore, config)
		} else if !errors.Is(err, os.ErrNotExist) {
			// the file does not exist if the container was created with nerdctl < 0.20
			return err
		}
		return nil
	}, nil
}
