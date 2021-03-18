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

package logging

import (
	"context"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/nerdctl/pkg/logging/jsonfile"
	"github.com/pkg/errors"
)

// MagicArgv1 is the magic argv1 for the containerd runtime v2 logging plugin mode.
const MagicArgv1 = "_NERDCTL_INTERNAL_LOGGING"

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

func getLoggerFunc(dataStore string) (logging.LoggerFunc, error) {
	if dataStore == "" {
		return nil, errors.New("got empty data store")
	}
	return func(_ context.Context, config *logging.Config, ready func() error) error {
		if config.Namespace == "" || config.ID == "" {
			return errors.New("got invalid config")
		}
		logJSONFilePath := jsonfile.Path(dataStore, config.Namespace, config.ID)
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
		return jsonfile.Encode(logJSONFile, config.Stdout, config.Stderr)
	}, nil
}
