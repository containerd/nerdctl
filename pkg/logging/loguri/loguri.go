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

// Package loguri answers questions about a container's log URI. It is a leaf on
// purpose: pkg/logging pulls in the log drivers and, through journald,
// pkg/containerutil, so the packages that build a container's task cannot
// import it.
package loguri

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// MagicArgv1 is the magic argv1 for the containerd runtime v2 logging plugin
// mode.
const MagicArgv1 = "_NERDCTL_INTERNAL_LOGGING"

// IsInternal reports whether uri makes containerd spawn *this* binary as
// nerdctl's logging process, which is what owns the container's stdio and
// serves attach sessions.
//
// Matching the magic argument alone is not enough. A container created with
// `--log-driver none` by a nerdctl that predates the broker has the literal URI
// "none", and one created with a custom `binary://` URI has somebody else's
// process there; neither can be brokered. Worse, a URI written by an older
// nerdctl points at wherever that binary was installed, which may still be an
// older nerdctl that knows nothing about the socket. Comparing the path against
// the running executable answers the question that actually matters: will
// spawning this URI run code that serves attach sessions?
//
// A capability marker in the URI query would be the other way to answer it, but
// the query becomes the logging process's argv and cmd/nerdctl/main.go
// dispatches that mode on exactly three arguments, so an extra parameter would
// break the logging plugin outright.
func IsInternal(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "binary" {
		return false
	}
	if u.Query().Get(MagicArgv1) == "" {
		return false
	}

	self, err := os.Executable()
	if err != nil {
		return false
	}
	// Resolve both sides: the URI records the path as it was at creation time,
	// which may reach the same binary through a different symlink.
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return false
	}
	target, err := filepath.EvalSymlinks(BinaryPath(u))
	if err != nil {
		return false
	}
	return target == self
}

// BinaryPath returns the executable a binary log URI names.
//
// cio.LogURIGenerator always gives the path a leading slash, so that a Windows
// path does not look like a host name. That slash has to come back off before
// the path means anything to the filesystem, which is what
// taskutil.terminalBrokerIO does too.
func BinaryPath(u *url.URL) string {
	if runtime.GOOS == "windows" {
		return strings.TrimPrefix(u.Path, "/")
	}
	return u.Path
}

// DataStore returns the data store recorded in an internal log URI, or an empty
// string if there is none.
//
// This is the value containerd's shim passes to the logging process as its
// argv, and therefore the exact string that process uses to derive the paths it
// shares with its clients: the attach socket and the stdin FIFO. Clients read
// it here rather than resolving the data store again, so the two sides cannot
// disagree over a symlink or a differently spelled --data-root.
func DataStore(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Query().Get(MagicArgv1)
}
