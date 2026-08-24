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

package netutil

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

// The Windows "nat" CNI plugin has no separate portmap plugin to chain (unlike
// the Linux bridge driver), so it must advertise portMappings/dns capabilities
// itself, or `nerdctl run -p` port mappings are silently ignored by the plugin.
func TestNewNatPluginCapabilities(t *testing.T) {
	nat := newNatPlugin("Ethernet")

	assert.Equal(t, nat.Capabilities["portMappings"], true)
	assert.Equal(t, nat.Capabilities["dns"], true)

	b, err := json.Marshal(nat)
	assert.NilError(t, err)

	var decoded map[string]interface{}
	assert.NilError(t, json.Unmarshal(b, &decoded))

	capabilities, ok := decoded["capabilities"].(map[string]interface{})
	assert.Assert(t, ok, "expected capabilities field in generated nat plugin config")
	assert.Equal(t, capabilities["portMappings"], true)
	assert.Equal(t, capabilities["dns"], true)
}
