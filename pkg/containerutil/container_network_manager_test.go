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

package containerutil

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestZeroMapValues(t *testing.T) {
	emptyString := ""
	testCases := []struct {
		key          string
		value        interface{}
		shouldBeZero bool
	}{
		{
			key:          "false",
			value:        false,
			shouldBeZero: true,
		},
		{
			key:          "true",
			value:        true,
			shouldBeZero: false,
		},
		{
			key:          "zeroInt",
			value:        int(0),
			shouldBeZero: true,
		},
		{
			key:          "nonZeroInt",
			value:        int(1),
			shouldBeZero: false,
		},
		{
			key:          "zeroString",
			value:        "",
			shouldBeZero: true,
		},
		{
			key:          "nonZeroString",
			value:        "non-zero",
			shouldBeZero: false,
		},
		{
			key:          "nilPointer",
			value:        (*string)(nil),
			shouldBeZero: true,
		},
		{
			key:   "pointerToEmpty",
			value: &emptyString,
			// technically just a nil pointer check, so any value should be non-Zero:
			shouldBeZero: false,
		},
		{
			key:          "nilSlice",
			value:        []string(nil),
			shouldBeZero: true,
		},
		{
			key:          "emptySlice",
			value:        []string{},
			shouldBeZero: false,
		},
		{
			key:          "nonEmptySlice",
			value:        []string{"non-empty"},
			shouldBeZero: false,
		},
		{
			key:          "nilMap",
			value:        map[string]int(nil),
			shouldBeZero: true,
		},
		{
			key:          "emptyMap",
			value:        map[string]int{},
			shouldBeZero: false,
		},
		{
			key:          "nonEmptyMap",
			value:        map[string]int{"non-empty": 42},
			shouldBeZero: false,
		},
	}

	for _, tc := range testCases {
		testName := fmt.Sprintf("%s=%t", tc.key, tc.shouldBeZero)
		t.Run(testName, func(tt *testing.T) {
			result := nonZeroMapValues(map[string]interface{}{tc.key: tc.value})
			if tc.shouldBeZero {
				assert.Equal(tt, len(result), 0)
			} else {
				assert.Equal(tt, len(result), 1)
			}
		})
	}
}
