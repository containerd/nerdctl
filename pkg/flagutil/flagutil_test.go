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

package flagutil

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// tmpFileWithContent will create a temp file with given content.
func tmpFileWithContent(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "flagutil-test")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpFile.Close()

	_, err = tmpFile.WriteString(content)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(tmpFile.Name())
	})
	return tmpFile.Name()
}

func TestReplaceOrAppendEnvValues(t *testing.T) {
	tests := []struct {
		defaults  []string
		overrides []string
		expected  []string
	}{
		// override defaults
		{
			defaults:  []string{"A=default", "B=default"},
			overrides: []string{"A=override", "C=override"},
			expected:  []string{"A=override", "B=default", "C=override"},
		},
		// empty defaults
		{
			defaults:  []string{"A=default", "B=default"},
			overrides: []string{"A=override", "B="},
			expected:  []string{"A=override", "B="},
		},
		// remove defaults
		{
			defaults:  []string{"A=default", "B=default"},
			overrides: []string{"A=override", "B"},
			expected:  []string{"A=override"},
		},
	}

	comparator := func(s1, s2 []string) bool {
		if len(s1) != len(s2) {
			return false
		}
		sort.Slice(s1, func(i, j int) bool {
			return s1[i] < s1[j]
		})
		sort.Slice(s2, func(i, j int) bool {
			return s2[i] < s2[j]
		})
		for i, v := range s1 {
			if v != s2[i] {
				return false
			}
		}
		return true
	}
	for _, tt := range tests {
		actual := ReplaceOrAppendEnvValues(tt.defaults, tt.overrides)
		assert.Assert(t, comparator(actual, tt.expected), fmt.Sprintf("expected: %s, actual: %s", tt.expected, actual))
	}
}

// Test TestParseEnvFileGoodFile for a env file with a few well formatted lines.
func TestParseEnvFileGoodFile(t *testing.T) {
	content := `foo=bar
    baz=quux
# comment

_foobar=foobaz
with.dots=working
and_underscore=working too`
	// Adding a newline + a line with pure whitespace.
	// This is being done like this instead of the block above
	// because it's common for editors to trim trailing whitespace
	// from lines, which becomes annoying since that's the
	// exact thing we need to test.
	content += "\n    \t  "
	tmpFile := tmpFileWithContent(t, content)

	lines, err := parseEnvVars([]string{tmpFile})
	if err != nil {
		t.Fatal(err)
	}

	expectedLines := []string{
		"foo=bar",
		"baz=quux",
		"_foobar=foobaz",
		"with.dots=working",
		"and_underscore=working too",
	}

	if !reflect.DeepEqual(lines, expectedLines) {
		t.Fatal("lines not equal to expectedLines")
	}
}

// Test TestParseEnvFileEmptyFile for an empty file.
func TestParseEnvFileEmptyFile(t *testing.T) {
	tmpFile := tmpFileWithContent(t, "")

	paths := []string{tmpFile}
	lines, err := parseEnvVars(paths)
	if err != nil {
		t.Fatal(err)
	}

	if len(lines) != 0 {
		t.Fatal("lines not empty; expected empty")
	}
}

// Test TestParseEnvFileNonExistentFile for a non existent file.
func TestParseEnvFileNonExistentFile(t *testing.T) {
	_, err := parseEnvVars([]string{"foo_bar_baz"})
	if err == nil {
		t.Fatal("ParseEnvFile succeeded; expected failure")
	}
	if _, ok := errors.Unwrap(err).(*os.PathError); !ok {
		t.Fatalf("Expected a PathError, got [%v]", err)
	}
}

// Test TestValidateEnv for the validate function's correctness.
func TestValidateEnv(t *testing.T) {
	type testCase struct {
		value    string
		expected string
		err      error
	}
	tests := []testCase{
		{
			value:    "a",
			expected: "a",
		},
		{
			value:    "something",
			expected: "something",
		},
		{
			value:    "_=a",
			expected: "_=a",
		},
		{
			value:    "env1=value1",
			expected: "env1=value1",
		},
		{
			value:    "_env1=value1",
			expected: "_env1=value1",
		},
		{
			value:    "env2=value2=value3",
			expected: "env2=value2=value3",
		},
		{
			value:    "env3=abc!qwe",
			expected: "env3=abc!qwe",
		},
		{
			value:    "env_4=value 4",
			expected: "env_4=value 4",
		},
		{
			value:    "PATH",
			expected: fmt.Sprintf("PATH=%v", os.Getenv("PATH")),
		},
		{
			value: "=a",
			err:   fmt.Errorf("invalid environment variable: =a"),
		},
		{
			value:    "PATH=",
			expected: "PATH=",
		},
		{
			value:    "PATH=something",
			expected: "PATH=something",
		},
		{
			value:    "asd!qwe",
			expected: "asd!qwe",
		},
		{
			value:    "1asd",
			expected: "1asd",
		},
		{
			value:    "123",
			expected: "123",
		},
		{
			value:    "some space",
			expected: "some space",
		},
		{
			value:    "  some space before",
			expected: "  some space before",
		},
		{
			value:    "some space after  ",
			expected: "some space after  ",
		},
		{
			value: "=",
			err:   fmt.Errorf("invalid environment variable: ="),
		},
	}

	if runtime.GOOS == "windows" {
		// Environment variables are case in-sensitive on Windows
		tests = append(tests, testCase{
			value:    "PaTh",
			expected: fmt.Sprintf("PaTh=%v", os.Getenv("PATH")),
			err:      nil,
		})
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.value, func(t *testing.T) {
			actual, err := withOSEnv([]string{tc.value})
			if tc.err == nil {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, tc.err.Error())
			}
			if actual != nil {
				v := actual[0]
				assert.Equal(t, v, tc.expected)
			}
		})
	}
}

// Test TestParseEnvFileLineTooLongFile for a file with a line exceeding bufio.MaxScanTokenSize.
func TestParseEnvFileLineTooLongFile(t *testing.T) {
	content := "foo=" + strings.Repeat("a", bufio.MaxScanTokenSize+42)
	tmpFile := tmpFileWithContent(t, content)

	_, err := MergeEnvFileAndOSEnv([]string{tmpFile}, nil)
	if err == nil {
		t.Fatal("ParseEnvFile succeeded; expected failure")
	}
}

// Test TestParseEnvVariableWithNoNameFile for parsing env file with empty variable name.
func TestParseEnvVariableWithNoNameFile(t *testing.T) {
	content := `# comment=
=blank variable names are an error case
`
	tmpFile := tmpFileWithContent(t, content)

	_, err := MergeEnvFileAndOSEnv([]string{tmpFile}, nil)
	if nil == err {
		t.Fatal("if a variable has no name parsing an environment file must fail")
	}
}

// Test TestMergeEnvFileAndOSEnv for merging variables from env-file and env.
func TestMergeEnvFileAndOSEnv(t *testing.T) {
	content := `HOME`
	tmpFile := tmpFileWithContent(t, content)

	variables, err := MergeEnvFileAndOSEnv([]string{tmpFile}, []string{"PATH"})
	if nil != err {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(variables) != 2 {
		t.Fatal("variables from env-file and flag env should be merged")
	}

	if "HOME="+os.Getenv("HOME") != variables[0] {
		t.Fatal("the HOME variable is not properly imported as the first variable")
	}

	if "PATH="+os.Getenv("PATH") != variables[1] {
		t.Fatal("the PATH variable is not properly imported as the second variable")
	}
}
