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

package volume

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestGetVolumeFilterFuncsRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		filter string
		want   string
	}{
		{"name", "bad format of filter"},
		{"label", "bad format of filter"},
		{"size", "bad format of filter"},
		{"names=foo", "invalid filter 'names'"},
		{"dangling=true", "invalid filter 'dangling'"},
		{"driver=local", "invalid filter 'driver'"},
		{"sizefoo=1", "invalid filter 'sizefoo'"},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			_, _, _, _, err := getVolumeFilterFuncs([]string{tc.filter})
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
