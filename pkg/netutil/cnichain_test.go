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
	"testing"

	"gotest.tools/v3/assert"
)

func TestFormatCNIChainName(t *testing.T) {
	// Expected values are generated with FormatChainName in
	// github.com/containernetworking/plugins/pkg/utils v1.10.0.
	assert.Equal(t, "CNI-fddae937f537b4ff49b6d0dd", FormatCNIChainName("bridge", "default-0123456789abcdef"))
	assert.Equal(t, "CNI-f7fbba6e0636f890e56fbbf3", FormatCNIChainName("foo", ""))
}
