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

func TestProcessSplit(t *testing.T) {
	tests := []struct {
		res      Processed
		src, dst string
		options  []string
		s        string
		wants    []string
	}{
		{
			res:     Processed{},
			src:     "",
			dst:     "",
			s:       "C:\\ProgramData\\nerdctl\\test\\volumes\\default\\testVol\\_data:C:\\src",
			options: []string{},
		},
		{
			res:     Processed{},
			src:     "",
			dst:     "",
			s:       "C:\\ProgramData\\nerdctl\\test\\volumes\\default\\Volmnt:C:\\src",
			options: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			src, dst, _, err := ProcessSplit(tt.s, nil, &tt.res, tt.src, tt.dst, tt.options)
			if err != nil {
				t.Errorf("failed to split %q: %v", tt.s, err)
				return
			}
			if !strings.Contains(tt.s, src) {
				t.Errorf("wrong volume src path %q & src %s", tt.s, src)
				return
			}
			if !strings.Contains(tt.s, dst) {
				t.Errorf("wrong volume dst path %q", tt.s)
				return
			}
		})
	}
}
