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
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/muesli/cancelreader"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/lockutil"
)

const (
	// MagicArgv1 is the magic argv1 for the containerd runtime v2 logging plugin mode.
	MagicArgv1 = "_NERDCTL_INTERNAL_LOGGING"
	LogPath    = "log-path"
	MaxSize    = "max-size"
	MaxFile    = "max-file"
	Tag        = "tag"
	Env        = "env"
	Labels     = "labels"
)

type Driver interface {
	Init(dataStore, ns, id string) error
	PreProcess(ctx context.Context, dataStore string, config *logging.Config) error
	Process(stdout <-chan string, stderr <-chan string) error
	PostProcess() error
}

type DriverFactory func(map[string]string, string) (Driver, error)
type LogOptsValidateFunc func(logOptMap map[string]string) error

var drivers = make(map[string]DriverFactory)
var driversLogOptsValidateFunctions = make(map[string]LogOptsValidateFunc)

func ValidateLogOpts(logDriver string, logOpts map[string]string) error {
	if value, ok := driversLogOptsValidateFunctions[logDriver]; ok && value != nil {
		return value(logOpts)
	}
	return nil
}

func RegisterDriver(name string, f DriverFactory, validateFunc LogOptsValidateFunc) {
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

func GetDriver(name string, opts map[string]string, address string) (Driver, error) {
	driverFactory, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown logging driver %q: %w", name, errdefs.ErrNotFound)
	}
	return driverFactory(opts, address)
}

func init() {
	RegisterDriver("none", func(opts map[string]string, address string) (Driver, error) {
		return &NoneLogger{}, nil
	}, NoneLogOptsValidate)
	RegisterDriver("json-file", func(opts map[string]string, address string) (Driver, error) {
		return &JSONLogger{Opts: opts}, nil
	}, JSONFileLogOptsValidate)
	RegisterDriver("journald", func(opts map[string]string, address string) (Driver, error) {
		return &JournaldLogger{Opts: opts, Address: address}, nil
	}, JournalLogOptsValidate)
	RegisterDriver("fluentd", func(opts map[string]string, address string) (Driver, error) {
		return &FluentdLogger{Opts: opts}, nil
	}, FluentdLogOptsValidate)
	RegisterDriver("syslog", func(opts map[string]string, address string) (Driver, error) {
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
	Driver  string            `json:"driver"`
	Opts    map[string]string `json:"opts,omitempty"`
	LogURI  string            `json:"-"`
	Address string            `json:"address"`
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
		return logConfig, fmt.Errorf("failed to read log config file %q: %w", logConfigFilePath, err)
	}

	err = json.Unmarshal(logConfigData, &logConfig)
	if err != nil {
		return logConfig, fmt.Errorf("failed to load JSON logging config file %q: %w", logConfigFilePath, err)
	}
	return logConfig, nil
}

func getLockPath(dataStore, ns, id string) string {
	return filepath.Join(dataStore, "containers", ns, id, "logger-lock")
}

// WaitForLogger waits until the logger has finished executing and processing container logs
func WaitForLogger(dataStore, ns, id string) error {
	return lockutil.WithDirLock(getLockPath(dataStore, ns, id), func() error {
		return nil
	})
}

func getContainerWait(ctx context.Context, address string, config *logging.Config) (<-chan containerd.ExitStatus, error) {
	client, err := containerd.New(strings.TrimPrefix(address, "unix://"), containerd.WithDefaultNamespace(config.Namespace))
	if err != nil {
		return nil, err
	}
	con, err := client.LoadContainer(ctx, config.ID)
	if err != nil {
		return nil, err
	}

	task, err := con.Task(ctx, nil)
	if err == nil {
		return task.Wait(ctx)
	}
	if !errdefs.IsNotFound(err) {
		return nil, err
	}

	// If task was not found, it's possible that the container runtime is still being created.
	// Retry every 100ms.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timed out waiting for container task to start")
		case <-ticker.C:
			task, err = con.Task(ctx, nil)
			if err != nil {
				if errdefs.IsNotFound(err) {
					continue
				}
				return nil, err
			}
			return task.Wait(ctx)
		}
	}
}

type ContainerWaitFunc func(ctx context.Context, address string, config *logging.Config) (<-chan containerd.ExitStatus, error)

func loggingProcessAdapter(ctx context.Context, driver Driver, dataStore, address string, getContainerWait ContainerWaitFunc, config *logging.Config) error {
	if err := driver.PreProcess(ctx, dataStore, config); err != nil {
		return err
	}

	stdoutR, err := cancelreader.NewReader(config.Stdout)
	if err != nil {
		return err
	}
	stderrR, err := cancelreader.NewReader(config.Stderr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done() // delivered on SIGTERM
		stdoutR.Cancel()
		stderrR.Cancel()
	}()

	// initialize goroutines to copy stdout and stderr streams to a closable pipe
	pipeStdoutR, pipeStdoutW := io.Pipe()
	pipeStderrR, pipeStderrW := io.Pipe()
	copyStream := func(reader io.Reader, writer *io.PipeWriter) {
		// copy using a buffer of size 32K
		buf := make([]byte, 32<<10)
		_, err := io.CopyBuffer(writer, reader, buf)
		if err != nil {
			log.G(ctx).Errorf("failed to copy stream: %s", err)
		}
	}
	go copyStream(stdoutR, pipeStdoutW)
	go copyStream(stderrR, pipeStderrW)

	var wg sync.WaitGroup
	wg.Add(3)
	stdout := make(chan string, 10000)
	stderr := make(chan string, 10000)
	processLogFunc := func(reader io.Reader, dataChan chan string) {
		defer wg.Done()
		defer close(dataChan)
		r := bufio.NewReader(reader)

		var err error

		for err == nil {
			var s string
			s, err = r.ReadString('\n')
			if len(s) > 0 {
				dataChan <- strings.TrimSuffix(s, "\n")
			}

			if err != nil && err != io.EOF {
				log.L.WithError(err).Error("failed to read log")
			}
		}
	}
	go processLogFunc(pipeStdoutR, stdout)
	go processLogFunc(pipeStderrR, stderr)
	go func() {
		defer wg.Done()
		driver.Process(stdout, stderr)
	}()
	go func() {
		// close pipeStdoutW and pipeStderrW upon container exit
		defer pipeStdoutW.Close()
		defer pipeStderrW.Close()

		exitCh, err := getContainerWait(ctx, address, config)
		if err != nil {
			log.G(ctx).Errorf("failed to get container task wait channel: %v", err)
			return
		}
		<-exitCh
	}()
	wg.Wait()
	return driver.PostProcess()
}

func loggerFunc(dataStore string) (logging.LoggerFunc, error) {
	if dataStore == "" {
		return nil, errors.New("got empty data store")
	}
	return func(ctx context.Context, config *logging.Config, ready func() error) error {
		if config.Namespace == "" || config.ID == "" {
			return errors.New("got invalid config")
		}
		logConfigFilePath := LogConfigFilePath(dataStore, config.Namespace, config.ID)
		if _, err := os.Stat(logConfigFilePath); err == nil {
			logConfig, err := LoadLogConfig(dataStore, config.Namespace, config.ID)
			if err != nil {
				return err
			}
			driver, err := GetDriver(logConfig.Driver, logConfig.Opts, logConfig.Address)
			if err != nil {
				return err
			}

			loggerLock := getLockPath(dataStore, config.Namespace, config.ID)
			f, err := os.Create(loggerLock)
			if err != nil {
				return err
			}
			defer f.Close()

			// the logger will obtain an exclusive lock on a file until the container is
			// stopped and the driver has finished processing all output,
			// so that waiting log viewers can be signalled when the process is complete.
			return lockutil.WithDirLock(loggerLock, func() error {
				if err := ready(); err != nil {
					return err
				}
				// getContainerWait is extracted as parameter to allow mocking in tests.
				return loggingProcessAdapter(ctx, driver, dataStore, logConfig.Address, getContainerWait, config)
			})
		} else if !errors.Is(err, os.ErrNotExist) {
			// the file does not exist if the container was created with nerdctl < 0.20
			return err
		}
		return nil
	}, nil
}

func NewLogFileWatcher(dir string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %v", err)
	}
	if err = watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch directory %q: %w", dir, err)
	}
	return watcher, nil
}

// startTail wait for the next log write.
// the boolean value indicates if the log file was recreated;
// the error is error happens during waiting new logs.
func startTail(ctx context.Context, logName string, w *fsnotify.Watcher) (bool, error) {
	errRetry := 5
	for {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("context cancelled")
		case e := <-w.Events:
			switch {
			case e.Has(fsnotify.Write):
				return false, nil
			case e.Has(fsnotify.Create):
				return filepath.Base(e.Name) == logName, nil
			default:
				log.L.Debugf("Received unexpected fsnotify event: %v, retrying", e)
			}
		case err := <-w.Errors:
			log.L.WithError(err).Debugf("Received fsnotify watch error, retrying unless no more retries left, retries: %d", errRetry)
			if errRetry == 0 {
				return false, err
			}
			errRetry--
		case <-time.After(logForceCheckPeriod):
			return false, nil
		}
	}
}
