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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/log"
)

const (
	// MagicArgv1 is the magic argv1 for the containerd runtime v2 logging plugin mode.
	MagicArgv1 = "_NERDCTL_INTERNAL_LOGGING"
	LogPath    = "log-path"
	MaxSize    = "max-size"
	MaxFile    = "max-file"
	Tag        = "tag"
)

type Driver interface {
	Init(dataStore, ns, id string) error
	PreProcess(dataStore string, config *logging.Config) error
	Process(stdout <-chan string, stderr <-chan string) error
	PostProcess() error
}

type DriverFactory func(map[string]string) (Driver, error)
type LogOpsValidateFunc func(logOptMap map[string]string) error

var drivers = make(map[string]DriverFactory)
var driversLogOptsValidateFunctions = make(map[string]LogOpsValidateFunc)

func ValidateLogOpts(logDriver string, logOpts map[string]string) error {
	if value, ok := driversLogOptsValidateFunctions[logDriver]; ok && value != nil {
		return value(logOpts)
	}
	return nil
}

func RegisterDriver(name string, f DriverFactory, validateFunc LogOpsValidateFunc) {
	drivers[name] = f
	driversLogOptsValidateFunctions[name] = validateFunc
}

func Drivers() []string {
	var ss []string // nolint: prealloc
	for f := range drivers {
		ss = append(ss, f)
	}
	sort.Strings(ss)
	return ss
}

func GetDriver(name string, opts map[string]string) (Driver, error) {
	driverFactory, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown logging driver %q: %w", name, errdefs.ErrNotFound)
	}
	return driverFactory(opts)
}

func init() {
	RegisterDriver("json-file", func(opts map[string]string) (Driver, error) {
		return &JSONLogger{Opts: opts}, nil
	}, JSONFileLogOptsValidate)
	RegisterDriver("journald", func(opts map[string]string) (Driver, error) {
		return &JournaldLogger{Opts: opts}, nil
	}, JournalLogOptsValidate)
	RegisterDriver("fluentd", func(opts map[string]string) (Driver, error) {
		return &FluentdLogger{Opts: opts}, nil
	}, FluentdLogOptsValidate)
	RegisterDriver("syslog", func(opts map[string]string) (Driver, error) {
		return &SyslogLogger{Opts: opts}, nil
	}, SyslogOptsValidate)
}

// Main is the entrypoint for the containerd runtime v2 logging plugin mode.
//
// Should be called only if argv1 == MagicArgv1.
func Main(argv2 string) error {
	fn, err := loggerFunc(argv2)
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
	LogURI string            `json:"-"`
}

// LogConfigFilePath returns the path of log-config.json
func LogConfigFilePath(dataStore, ns, id string) string {
	return filepath.Join(dataStore, "containers", ns, id, "log-config.json")
}

// LoadLogConfig loads the log-config.json for the afferrent container store
func LoadLogConfig(dataStore, ns, id string) (LogConfig, error) {
	logConfig := LogConfig{}

	logConfigFilePath := LogConfigFilePath(dataStore, ns, id)
	logConfigData, err := os.ReadFile(logConfigFilePath)
	if err != nil {
		return logConfig, fmt.Errorf("failed to read log config file %q: %s", logConfigFilePath, err)
	}

	err = json.Unmarshal(logConfigData, &logConfig)
	if err != nil {
		return logConfig, fmt.Errorf("failed to load JSON logging config file %q: %s", logConfigFilePath, err)
	}
	return logConfig, nil
}

func loggingProcessAdapter(driver Driver, dataStore string, config *logging.Config) error {
	if err := driver.PreProcess(dataStore, config); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(3)
	stdout := make(chan string, 10000)
	stderr := make(chan string, 10000)
	processLogFunc := func(reader io.Reader, dataChan chan string) {
		defer wg.Done()
		defer close(dataChan)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			if scanner.Err() != nil {
				log.L.Errorf("failed to read log: %v", scanner.Err())
				return
			}
			dataChan <- scanner.Text()
		}
	}

	go processLogFunc(config.Stdout, stdout)
	go processLogFunc(config.Stderr, stderr)
	go func() {
		defer wg.Done()
		driver.Process(stdout, stderr)
	}()
	wg.Wait()
	return driver.PostProcess()
}

func loggerFunc(dataStore string) (logging.LoggerFunc, error) {
	if dataStore == "" {
		return nil, errors.New("got empty data store")
	}
	return func(_ context.Context, config *logging.Config, ready func() error) error {
		if config.Namespace == "" || config.ID == "" {
			return errors.New("got invalid config")
		}
		logConfigFilePath := LogConfigFilePath(dataStore, config.Namespace, config.ID)
		if _, err := os.Stat(logConfigFilePath); err == nil {
			logConfig, err := LoadLogConfig(dataStore, config.Namespace, config.ID)
			if err != nil {
				return err
			}
			driver, err := GetDriver(logConfig.Driver, logConfig.Opts)
			if err != nil {
				return err
			}
			if err := ready(); err != nil {
				return err
			}

			return loggingProcessAdapter(driver, dataStore, config)
		} else if !errors.Is(err, os.ErrNotExist) {
			// the file does not exist if the container was created with nerdctl < 0.20
			return err
		}
		return nil
	}, nil
}
