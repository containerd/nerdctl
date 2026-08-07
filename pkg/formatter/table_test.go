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

package formatter

import (
	"bytes"
	"testing"

	"gotest.tools/v3/assert"
)

func TestIsTableFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		format   string
		expected bool
	}{
		{format: "table", expected: true},
		{format: `table {{.Type}}`, expected: true},
		{format: `table {{.Type}}\t{{.Size}}`, expected: true},
		{format: "", expected: false},
		{format: "json", expected: false},
		{format: `{{.Type}}`, expected: false},
		// A template that merely starts with the word is not a table format.
		{format: `{{.Type}} table`, expected: false},
		{format: "tabled", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, IsTableFormat(tc.format), tc.expected)
		})
	}
}

func TestParseTableTemplate(t *testing.T) {
	t.Parallel()

	// The shell passes \t and \n through literally, so they arrive as two characters and have to
	// be expanded, the way docker/cli does.
	rows, _, err := ParseTableTemplate(`table {{.A}}\t{{.B}}\n`)
	assert.NilError(t, err)

	var b bytes.Buffer
	err = rows.Execute(&b, struct{ A, B string }{A: "one", B: "two"})
	assert.NilError(t, err)
	assert.Equal(t, b.String(), "one\ttwo\n")
}

func TestParseTableTemplateHeader(t *testing.T) {
	t.Parallel()

	// The functions that transform a value must leave the column labels alone, otherwise the
	// header of `table {{lower .A}}` would read "a" instead of naming the column.
	rows, header, err := ParseTableTemplate(`table {{lower .A}}\t{{truncate .B 2}}\t{{upper .C}}`)
	assert.NilError(t, err)

	type row struct{ A, B, C string }

	var headerOut bytes.Buffer
	err = header.Execute(&headerOut, row{A: "NAME", B: "SIZE", C: "Status"})
	assert.NilError(t, err)
	assert.Equal(t, headerOut.String(), "NAME\tSIZE\tStatus")

	// The rows themselves still go through the functions they asked for.
	var rowOut bytes.Buffer
	err = rows.Execute(&rowOut, row{A: "Foo", B: "100B", C: "up"})
	assert.NilError(t, err)
	assert.Equal(t, rowOut.String(), "foo\t10\tUP")
}

func TestParseTableTemplateInvalid(t *testing.T) {
	t.Parallel()

	_, _, err := ParseTableTemplate(`table {{.Unclosed`)
	assert.Assert(t, err != nil)
}
