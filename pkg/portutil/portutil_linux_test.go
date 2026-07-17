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

package portutil

import (
	"fmt"
	"net"
	"testing"

	"gotest.tools/v3/assert"
)

// TestParseFlagPHostRangePool verifies the Docker-compatible behavior for a single
// container port with a host port range (e.g. "3000-3001:8080"): the container port
// is bound to one free host port from the range, not collapsed-and-dropped. The test
// occupies the first port of a two-port range and asserts that the container port is
// bound to the next free host port (first+1); without the pool-allocation fix it would
// be dropped onto the occupied first port and this assertion would fail.
//
// This lives in a _linux_test.go file because getUsedPorts is only implemented on Linux.
func TestParseFlagPHostRangePool(t *testing.T) {
	// Occupy the first port of a two-port range and confirm that the single
	// container port is bound to the next free host port, not collapsed onto the
	// occupied first port. Without the pool fix this asserts the wrong port.
	var occupied net.Listener
	var first int
	for attempt := 0; attempt < 50; attempt++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		p := l.Addr().(*net.TCPAddr).Port
		if p+1 > 65535 {
			l.Close()
			continue
		}
		// Ensure the successor port is currently free.
		probe, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+1))
		if err != nil {
			l.Close()
			continue
		}
		probe.Close()
		occupied, first = l, p
		break
	}
	if occupied == nil {
		t.Fatal("could not find an occupied port with a free successor")
	}
	defer occupied.Close()

	got, err := ParseFlagP(fmt.Sprintf("127.0.0.1:%d-%d:8080/tcp", first, first+1))
	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].ContainerPort, int32(8080))
	assert.Equal(t, got[0].Protocol, "tcp")
	assert.Equal(t, got[0].HostIP, "127.0.0.1")
	assert.Equal(t, got[0].HostPort, int32(first+1))
}
