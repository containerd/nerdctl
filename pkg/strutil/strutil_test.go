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

package strutil

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestDedupeStrSlice(t *testing.T) {
	assert.DeepEqual(t,
		[]string{"apple", "banana", "chocolate"},
		DedupeStrSlice([]string{"apple", "banana", "apple", "chocolate"}))

	assert.DeepEqual(t,
		[]string{"apple", "banana", "chocolate"},
		DedupeStrSlice([]string{"apple", "apple", "banana", "chocolate", "apple"}))

}

func TestParseCSVMap(t *testing.T) {
	cases := map[string]map[string]string{
		`foo=x,bar=y,baz=z,qux`: {
			"foo": "x",
			"bar": "y",
			"baz": "z",
			"qux": "",
		},
		`"foo=x,bar=y",baz=z,qux`: {
			"foo": "x,bar=y",
			"baz": "z",
			"qux": "",
		},
	}

	for s, expected := range cases {
		got, err := ParseCSVMap(s)
		assert.NilError(t, err)
		assert.DeepEqual(t, expected, got)
	}
}
