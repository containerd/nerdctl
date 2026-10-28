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

package compose

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestParseScaleArgs(t *testing.T) {
	scale, err := parseScaleArgs([]string{"web=3", "db=0"})
	assert.NilError(t, err)
	assert.DeepEqual(t, scale, map[string]int{"web": 3, "db": 0})

	for _, arg := range []string{"web", "=3", "web=", "web=x", "web=-1"} {
		_, err := parseScaleArgs([]string{arg})
		assert.Assert(t, err != nil, "expected an error for %q", arg)
	}
}
