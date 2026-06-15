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

package maputil

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestMapBoolValueAsOpt(t *testing.T) {
	tests := []struct {
		m       map[string]string
		key     string
		want    bool
		wantErr bool
	}{
		// cases that key does not exist
		{
			m:       map[string]string{},
			key:     "key",
			want:    false,
			wantErr: false,
		},
		// key exist, but has no value
		{
			m:       map[string]string{"key": ""},
			key:     "key",
			want:    true,
			wantErr: false,
		},
		// key exist, and set to true
		{
			m:       map[string]string{"key": "true"},
			key:     "key",
			want:    true,
			wantErr: false,
		},
		// key exist, and set to false
		{
			m:       map[string]string{"key": "false"},
			key:     "key",
			want:    false,
			wantErr: false,
		},
		// cases with error
		{
			m:       map[string]string{"key": "abc"},
			key:     "key",
			want:    false,
			wantErr: true,
		},
	}

	for i, tt := range tests {
		got, err := MapBoolValueAsOpt(tt.m, tt.key)
		assert.Equal(t, got, tt.want, fmt.Sprintf("case %d", (i+1)))
		if (err != nil) != tt.wantErr {
			t.Errorf("MapBoolValueAsOpt() case %d error = %v, wantErr %v", (i + 1), err, tt.wantErr)
		}
	}
}
