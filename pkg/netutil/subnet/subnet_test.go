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

package subnet

import (
	"net"
	"testing"

	"gotest.tools/v3/assert"
)

func TestNextSubnet(t *testing.T) {
	testCases := []struct {
		subnet string
		expect string
	}{
		{
			subnet: "10.4.1.0/24",
			expect: "10.4.2.0/24",
		},
		{
			subnet: "10.4.255.0/24",
			expect: "10.5.0.0/24",
		},
		{
			subnet: "10.4.255.0/16",
			expect: "10.5.0.0/16",
		},
	}
	for _, tc := range testCases {
		_, net, _ := net.ParseCIDR(tc.subnet)
		nextSubnet, err := nextSubnet(net)
		assert.NilError(t, err)
		assert.Equal(t, nextSubnet.String(), tc.expect)
	}
}
