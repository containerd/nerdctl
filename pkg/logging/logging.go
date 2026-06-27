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
	"bytes"
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
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/muesli/cancelreader"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
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

const (
	streamStdout = "stdout"
	streamStderr = "stderr"
)

type Driver interface {
	Init(dataStore, ns, id string) error
	PreProcess(ctx context.Context, dataStore string, config *logging.Config) error
	Process(stdout <-chan string, stderr <-chan string) error
	PostProcess() error
}

// SyncDriver is an optional capability for a Driver whose log entries can be
// written synchronously and cheaply (e.g. to a local file). When a driver
// implements it, the logger writes each entry by calling WriteLogEntry directly
// from the goroutine that reads the container's stdio, rather than handing it to
// Process over a buffered channel. This makes a container's final output (in
// particular a trailing chunk with no newline) durable before containerd tears
// the logging process down on exit. Drivers that may block, such as
// network-backed ones, should not implement it so that the buffered channel
// keeps them from blocking the container. https://github.com/containerd/nerdctl/issues/5006
type SyncDriver interface {
	Driver
	// WriteLogEntry writes a single log line for the given stream ("stdout" or
	// "stderr"). It may be called concurrently for different streams.
	WriteLogEntry(stream, line string) error
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
	logConfigData, err := filesystem.ReadFile(logConfigFilePath)
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
	return filesystem.WithLock(getLockPath(dataStore, ns, id), func() error {
		return nil
	})
}

// alreadyExited returns a channel that immediately reports an exit, used when
// we have determined that the container's task is already gone.
func alreadyExited() <-chan containerd.ExitStatus {
	ch := make(chan containerd.ExitStatus, 1)
	ch <- containerd.ExitStatus{}
	return ch
}

func getContainerWait(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error) {
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

	// The task was not found. containerd starts this logging process while
	// setting up the container's IO, i.e. before the task is created, so a
	// NotFound here usually just means the task has not been created yet: retry
	// until it appears.
	//
	// However, for a short-lived container the task may instead have already
	// exited and been removed before we ever observed it (this is more likely
	// when this logger process is slow to start, e.g. under gomodjail). In that
	// case the task will never appear and waiting for it would hang the logger
	// forever, holding the logger lock and truncating the container's final
	// output. Once we have seen the container produce output we therefore know
	// it has run, so a still-missing task means it has already exited.
	// https://github.com/containerd/nerdctl/issues/5006
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timed out waiting for container task to start")
		case <-ticker.C:
			task, err = con.Task(ctx, nil)
			if err == nil {
				return task.Wait(ctx)
			}
			if !errdefs.IsNotFound(err) {
				return nil, err
			}
			if outputSeen() {
				return alreadyExited(), nil
			}
		}
	}
}

type ContainerWaitFunc func(ctx context.Context, address string, config *logging.Config, outputSeen func() bool) (<-chan containerd.ExitStatus, error)

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

	// copiedBytes counts how much container output has been read so far. It lets
	// getContainerWait tell "the task has not been created yet" apart from "the
	// task has already exited and been removed" when it sees a missing task.
	var copiedBytes atomic.Int64
	outputSeen := func() bool { return copiedBytes.Load() > 0 }

	stdout := make(chan string, 10000)
	stderr := make(chan string, 10000)

	// If the driver can write synchronously, emit writes each log entry directly
	// from the goroutine that reads the container's stdio. Otherwise it hands the
	// entry to the driver's Process method over a buffered channel, which keeps a
	// slow (e.g. network-backed) driver from blocking the container.
	//
	// The synchronous path matters because, when a container exits, containerd
	// closes the stdio FIFOs and then tears the logging process down almost
	// immediately. Handing the final chunk to another goroutine to write races
	// that teardown and can lose a trailing chunk that has no newline; writing it
	// inline does not. https://github.com/containerd/nerdctl/issues/5006
	syncDriver, isSync := driver.(SyncDriver)
	emit := func(stream, line string) {
		if isSync {
			if err := syncDriver.WriteLogEntry(stream, line); err != nil {
				log.G(ctx).WithError(err).Error("failed to write log entry")
			}
			return
		}
		if stream == streamStdout {
			stdout <- line
		} else {
			stderr <- line
		}
	}

	var wg sync.WaitGroup

	// processStream reads a container stdio FIFO directly and emits its output
	// split into newline-terminated lines. Complete lines are emitted as they are
	// read; a trailing fragment without a newline is buffered until more output
	// arrives (so a long line is not split) and emitted when the stream ends.
	processStream := func(stream string, reader io.Reader, dataChan chan string) {
		defer wg.Done()
		if !isSync {
			defer close(dataChan)
		}
		buf := make([]byte, 32<<10)
		var pending []byte
		// emitLines emits each complete (newline-terminated) line, leaving any
		// trailing fragment buffered in pending so that a single logical line is
		// not split across log entries.
		emitLines := func() {
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				emit(stream, string(pending[:i+1]))
				pending = pending[i+1:]
			}
		}
		for {
			nr, err := reader.Read(buf)
			if nr > 0 {
				copiedBytes.Add(int64(nr))
				pending = append(pending, buf[:nr]...)
				emitLines()
				// For a synchronous driver, emit a trailing fragment immediately
				// instead of buffering it until the stream ends. The fragment is
				// then written to the log before the container's abrupt teardown
				// on exit can lose it; for a streaming (channel) driver this would
				// only split lines, so it is left buffered there.
				if isSync && len(pending) > 0 {
					emit(stream, string(pending))
					pending = pending[:0]
				}
			}
			if err != nil {
				emitLines()
				// The stream has ended: emit any final fragment that did not end
				// in a newline.
				if len(pending) > 0 {
					emit(stream, string(pending))
				}
				if !errors.Is(err, io.EOF) && !errors.Is(err, cancelreader.ErrCanceled) {
					log.L.WithError(err).Error("failed to read log")
				}
				return
			}
		}
	}
	wg.Add(2)
	go processStream(streamStdout, stdoutR, stdout)
	go processStream(streamStderr, stderrR, stderr)
	if !isSync {
		wg.Add(1)
		go func() {
			defer wg.Done()
			driver.Process(stdout, stderr)
		}()
	}
	go func() {
		// Wait for the container to exit, then cancel the readers. containerd
		// keeps the stdio FIFO write ends open (so the container can be
		// restarted), so the FIFOs may not reach EOF on exit; without this the
		// read goroutines, and therefore the logger, could block forever.
		exitCh, err := getContainerWait(ctx, address, config, outputSeen)
		if err != nil {
			// We could not determine when the container exits. Do not cancel the
			// readers: they will finish on their own when the FIFO reaches EOF.
			// Cancelling here could truncate a still-running container.
			log.G(ctx).Errorf("failed to get container task wait channel: %v", err)
			return
		}
		<-exitCh
		stdoutR.Cancel()
		stderrR.Cancel()
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

			// the logger will obtain an exclusive lock on a file until the container is
			// stopped and the driver has finished processing all output,
			// so that waiting log viewers can be signalled when the process is complete.
			return filesystem.WithLock(loggerLock, func() error {
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
