//go:build !(linux || freebsd)

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

import "context"

// startBroker is a no-op on platforms without an attach transport. Attaching
// falls back to connecting directly to the container's stdio, which allows one
// session at a time.
//
// The build tag names the platforms rather than saying `unix`, because the
// stdin FIFO handling in broker_unix.go relies on Go registering a
// non-blocking FIFO with the runtime poller, and os/file_unix.go deliberately
// does not do that on darwin and ios (kqueue does not report the last writer
// closing a fifo, golang/go#24164). containerd does not run containers there
// anyway; this keeps `make lint-go-all` honest about it rather than shipping
// code that would return EAGAIN from a full FIFO.
func startBroker(context.Context, string, string, string, bool) (func(stream string, p []byte), func(exited bool)) {
	return nil, func(bool) {}
}
