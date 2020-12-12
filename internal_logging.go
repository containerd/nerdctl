/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const internalLoggingArgKey = "_NERDCTL_INTERNAL_LOGGING_ARG"

func internalLoggingMain(arg string) error {
	fn, err := getLoggerFunc(arg)
	if err != nil {
		return err
	}
	logging.Run(fn)
	return nil
}

func getLoggerFunc(dataRoot string) (logging.LoggerFunc, error) {
	if dataRoot == "" {
		return nil, errors.New("got empty data-root")
	}
	return func(_ context.Context, config *logging.Config, ready func() error) error {
		if config.Namespace == "" || config.ID == "" {
			return errors.New("got invalid config")
		}
		logJSONFilePath := getLogJSONPath(dataRoot, config.Namespace, config.ID)
		if err := os.MkdirAll(filepath.Dir(logJSONFilePath), 0700); err != nil {
			return err
		}
		logJSONFile, err := os.OpenFile(logJSONFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		defer logJSONFile.Close()
		if err := ready(); err != nil {
			return err
		}
		return writeLogJSON(logJSONFile, config.Stdout, config.Stderr)
	}, nil
}

func getLogJSONPath(dataRoot, ns, id string) string {
	// the file name corresponds to Docker
	return filepath.Join(dataRoot, "c", ns, id, id+"-json.log")
}

func writeLogJSON(w io.Writer, stdout, stderr io.Reader) error {
	enc := json.NewEncoder(w)
	var encMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)
	f := func(r io.Reader, name string) {
		defer wg.Done()
		br := bufio.NewReader(r)
		e := &LogJSONEntry{
			Stream: name,
		}
		for {
			line, err := br.ReadString(byte('\n'))
			if err != nil {
				logrus.WithError(err).Errorf("faild to read line from %q", name)
				return
			}
			e.Log = line
			e.Time = time.Now().UTC()
			encMu.Lock()
			encErr := enc.Encode(e)
			encMu.Unlock()
			if encErr != nil {
				logrus.WithError(err).Errorf("faild to encode JSON")
				return
			}
		}
	}
	go f(stdout, "stdout")
	go f(stderr, "stderr")
	wg.Wait()
	return nil
}

// LogJSONEntry is compatible with Docker, but not efficient
type LogJSONEntry struct {
	Log    string    `json:"log"` // line, including "\r\n"
	Stream string    `json:"stream"`
	Time   time.Time `json:"time"` // e.g. "2020-12-11T20:29:41.939902251Z"
}
