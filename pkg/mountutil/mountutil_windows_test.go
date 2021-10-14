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

package mountutil

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestParseVolumeOptions(t *testing.T) {
	tests := []struct {
		vType    string
		src      string
		optsRaw  string
		wants    []string
		wantFail bool
	}{
		{
			vType:   "bind",
			src:     "dummy",
			optsRaw: "rw",
			wants:   []string{},
		},
		{
			vType:   "volume",
			src:     "dummy",
			optsRaw: "ro",
			wants:   []string{"ro"},
		},
		{
			vType:   "volume",
			src:     "dummy",
			optsRaw: "ro,undefined",
			wants:   []string{"ro"},
		},
		{
			vType:    "bind",
			src:      "dummy",
			optsRaw:  "ro,rw",
			wantFail: true,
		},
		{
			vType:    "volume",
			src:      "dummy",
			optsRaw:  "ro,ro",
			wantFail: true,
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join([]string{tt.vType, tt.src, tt.optsRaw}, "-"), func(t *testing.T) {
			opts, _, err := parseVolumeOptions(tt.vType, tt.src, tt.optsRaw)
			if err != nil {
				if tt.wantFail {
					return
				}
				t.Errorf("failed to parse option %q: %v", tt.optsRaw, err)
				return
			}
			assert.Equal(t, tt.wantFail, false)
			assert.Check(t, is.DeepEqual(tt.wants, opts))
		})
	}
}
