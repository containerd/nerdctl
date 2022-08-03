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

package tabutil

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestTabReader(t *testing.T) {
	tabRows := strings.Split(`a    b    c
1    2    3
123  456  789`, "\n")
	reader := NewReader("a\tb\tc\t")

	err := reader.ParseHeader(tabRows[0])
	assert.NilError(t, err)

	var (
		value string
	)
	value, _ = reader.ReadRow(tabRows[1], "a")
	assert.Equal(t, value, "1")

	value, _ = reader.ReadRow(tabRows[1], "c")
	assert.Equal(t, value, "3")

	value, _ = reader.ReadRow(tabRows[2], "b")
	assert.Equal(t, value, "456")
}

func TestTabReaderParseTrimmedRow(t *testing.T) {
	// for example, row 2 and row 3 are trimmed because they do not contain value for the header c
	tabRows := strings.Split(`a    b    c
1    2
123  456`, "\n")
	reader := NewReader("a\tb\tc\t")

	err := reader.ParseHeader(tabRows[0])
	assert.NilError(t, err)

	var (
		value string
		ok    bool
	)
	value, ok = reader.ReadRow(tabRows[1], "a")
	assert.Equal(t, value, "1")
	assert.Equal(t, ok, true)
	value, ok = reader.ReadRow(tabRows[1], "b")
	assert.Equal(t, value, "2")
	assert.Equal(t, ok, true)
	value, ok = reader.ReadRow(tabRows[1], "c")
	assert.Equal(t, value, "")
	assert.Equal(t, ok, false)
	value, ok = reader.ReadRow(tabRows[2], "a")
	assert.Equal(t, value, "123")
	assert.Equal(t, ok, true)
	value, ok = reader.ReadRow(tabRows[2], "b")
	assert.Equal(t, value, "456")
	assert.Equal(t, ok, true)
	value, ok = reader.ReadRow(tabRows[2], "c")
	assert.Equal(t, value, "")
	assert.Equal(t, ok, false)
}
