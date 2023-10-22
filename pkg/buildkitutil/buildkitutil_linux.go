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

package buildkitutil

import (
	"fmt"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func getRuntimeVariableDataDir() string {
	// Per Linux Foundation "Filesystem Hierarchy Standard" version 3.0 section 3.15.
	// Under version 2.3, this was "/var/run".
	run := "/run"
	if rootlessutil.IsRootless() {
		var err error
		run, err = rootlessutil.XDGRuntimeDir()
		if err != nil {
			log.L.Warn(err)
			run = fmt.Sprintf("/run/user/%d", rootlessutil.ParentEUID())
		}
	}
	return run
}
