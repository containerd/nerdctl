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

package procnet

import (
	"net"
	"testing"

	"gotest.tools/v3/assert"
)

// All the code in this file is copied from the iima project in https://github.com/lima-vm/lima/blob/v0.8.3/pkg/guestagent/procnettcp/procnettcp_test.go.
// and is licensed under the Apache License, Version 2.0.

func TestParseTCP(t *testing.T) {
	procNetTCP := []string{
		"0: 0100007F:8AEF 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 28152 1 0000000000000000 100 0 0 10 0",
		"1: 0103000A:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 31474 1 0000000000000000 100 0 0 10 5",
		"2: 3500007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000   102        0 30955 1 0000000000000000 100 0 0 10 0",
		"3: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 32910 1 0000000000000000 100 0 0 10 0",
		"4: 0100007F:053A 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 31430 1 0000000000000000 100 0 0 10 0",
		"5: 0B3CA8C0:0016 690AA8C0:F705 01 00000000:00000000 02:00028D8B 00000000     0        0 32989 4 0000000000000000 20 4 31 10 19"}
	entries := Parse(procNetTCP)
	t.Log(entries)

	assert.Check(t, net.ParseIP("127.0.0.1").Equal(entries[0].LocalIP))
	assert.Equal(t, uint64(35567), entries[0].LocalPort)

	assert.Check(t, net.ParseIP("192.168.60.11").Equal(entries[5].LocalIP))
	assert.Equal(t, uint64(22), entries[5].LocalPort)
}

func TestParseTCP6(t *testing.T) {
	procNetTCP := []string{
		"0: 000080FE00000000FF57A6705DC771FE:0050 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 850222 1 0000000000000000 100 0 0 10 0",
	}
	entries := Parse(procNetTCP)
	t.Log(entries)

	assert.Check(t, net.ParseIP("fe80::70a6:57ff:fe71:c75d").Equal(entries[0].LocalIP))
	assert.Equal(t, uint64(80), entries[0].LocalPort)
}

func TestParseTCP6Zero(t *testing.T) {
	procNetTCP := []string{
		"0: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 33825 1 0000000000000000 100 0 0 10 0",
		"1: 00000000000000000000000000000000:006F 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 26772 1 0000000000000000 100 0 0 10 0",
		"2: 00000000000000000000000000000000:0050 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1210901 1 0000000000000000 100 0 0 10 0"}

	entries := Parse(procNetTCP)
	t.Log(entries)

	assert.Check(t, net.IPv6zero.Equal(entries[0].LocalIP))
	assert.Equal(t, uint64(22), entries[0].LocalPort)
}
