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

// portlock provides a mechanism for containers to acquire and release ports they plan to expose, and a wait mechanism
// This allows tests dependent on running containers to always parallelize without having to worry about port collision
// with any other test
// Note that this does NOT protect against trying to use a port that is already used by an unrelated third-party service or container
// Also note that *generally* finding a free port is not easy:
// - to just "listen" and see if it works won't work for containerized services that are DNAT-ed (plus, that would be racy)
// - inspecting iptables instead (or in addition to) may work for containers, but this depends on how networking has been set (and yes, it is also racy)
// Our approach here is optimistic: tests are responsible for calling Acquire and Release
package portlock

import (
	"fmt"
	"sync"
	"time"
)

var mut = &sync.Mutex{} //nolint:gochecknoglobals
var portList = map[int]bool{}

func Acquire(port int) (int, error) {
	flexible := false
	if port == 0 {
		port = 5000
		flexible = true
	}
	for {
		mut.Lock()
		if _, ok := portList[port]; !ok {
			portList[port] = true
			mut.Unlock()
			return port, nil
		}
		mut.Unlock()
		if flexible {
			port++
			continue
		}
		fmt.Println("Waiting for port to become available...", port)
		time.Sleep(1 * time.Second)
	}
}

func Release(port int) error {
	mut.Lock()
	delete(portList, port)
	mut.Unlock()
	return nil
}
