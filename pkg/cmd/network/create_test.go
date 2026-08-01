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

package network

import (
	"io"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

// TestCreateAuxAddressWithoutSubnet verifies that an aux-address given without
// any subnet is rejected the same way Docker rejects it, before any CNI setup.
func TestCreateAuxAddressWithoutSubnet(t *testing.T) {
	err := Create(types.NetworkCreateOptions{
		AuxAddresses: []string{"host=10.9.0.5"},
	}, io.Discard)
	assert.ErrorContains(t, err, "no matching subnet for aux-address 10.9.0.5")
}
