//go:build !linux

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

package signalutil

import "os"

// canIgnoreSignal is from https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/signals_notlinux.go#L23-L25
func canIgnoreSignal(_ os.Signal) bool {
	return false
}
