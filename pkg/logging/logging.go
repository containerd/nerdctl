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
	"github.com/containerd/containerd/runtime/v2/logging"
)

const (
	// MagicArgv1 is the magic argv1 for the containerd runtime v2 logging plugin mode.
	MagicArgv1 = "_NERDCTL_INTERNAL_LOGGING"
	MaxSize    = "max-size"
	MaxFile    = "max-file"
	DriverType = "logging-driver"
)

const (
	jsonFile = "json-file"
)

type NerdctlLoggingInterface interface {
	init(argsMap map[string]string) error
	getLoggerFunc(argsMap map[string]string) logging.LoggerFunc
}

// Main is the entrypoint for the containerd runtime v2 logging plugin mode.
//
// Should be called only if argv1 == MagicArgv1.
func Main(argsMap map[string]string) error {
	var driver NerdctlLoggingInterface
	// TODO: Impl more drivers such as local, journald, fluentd
	driverType := argsMap[DriverType]
	switch driverType {
	case jsonFile:
		driver = &JsonLogger{}
	default:
		return nil
	}
	if err := driver.init(argsMap); err != nil {
		return err
	}
	fn := driver.getLoggerFunc(argsMap)
	logging.Run(fn)
	return nil
}
