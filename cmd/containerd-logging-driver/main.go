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

package main

import (
	"flag"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/sirupsen/logrus"
)

var driverConfig logging.DriverConfig

func init() {
	flag.StringVar(&driverConfig.LogPath, "log-path", "", "The log path where the logs are written")
	flag.StringVar(&driverConfig.MaxSize, "max-size", "-1", "The maximum size of the log before it is rolled")
	flag.StringVar(&driverConfig.MaxFile, "max-file", "1", "The maximum number of log files that can be present")
	flag.StringVar(&driverConfig.Tag, "tag", "", "A string that is appended to the APP-NAME in the syslog message")
	flag.StringVar(&driverConfig.Driver, "driver", "", "A string that is appended to the APP-NAME in the syslog message")
}

func main() {
	flag.Parse()
	err := logging.Main(driverConfig)

	if err != nil {
		logrus.Fatal(err)
	}
}
