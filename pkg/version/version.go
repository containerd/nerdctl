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

package version

import (
	"runtime/debug"
	"strconv"
)

var (
	// Version is filled via Makefile
	Version = ""
	// Revision is filled via Makefile
	Revision = ""
)

const unknown = "<unknown>"

func GetVersion() string {
	if Version != "" {
		return Version
	}
	/*
	 * go install example.com/cmd/foo@vX.Y.Z: bi.Main.Version="vX.Y.Z",                               vcs.revision is unset
	 * go install example.com/cmd/foo@latest: bi.Main.Version="vX.Y.Z",                               vcs.revision is unset
	 * go install example.com/cmd/foo@master: bi.Main.Version="vX.Y.Z-N.yyyyMMddhhmmss-gggggggggggg", vcs.revision is unset
	 * go install ./cmd/foo:                  bi.Main.Version="(devel)", vcs.revision="gggggggggggggggggggggggggggggggggggggggg"
	 *                                        vcs.time="yyyy-MM-ddThh:mm:ssZ", vcs.modified=("false"|"true")
	 */
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return unknown
}

func GetRevision() string {
	if Revision != "" {
		return Revision
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		var (
			vcsRevision string
			vcsModified bool
		)
		for _, f := range bi.Settings {
			switch f.Key {
			case "vcs.revision":
				vcsRevision = f.Value
			case "vcs.modified":
				vcsModified, _ = strconv.ParseBool(f.Value)
			}
		}
		if vcsRevision == "" {
			return unknown
		}
		rev := vcsRevision
		if vcsModified {
			rev += ".m"
		}
		return rev
	}
	return unknown
}
