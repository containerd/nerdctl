//go:build freebsd || linux
// +build freebsd linux

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

package netutil

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"gotest.tools/v3/assert"
)

func TestGuessFirewallPluginVersion(t *testing.T) {

	type testCase struct {
		stderr   string
		expected string
		err      string
	}
	testCases := []testCase{
		{
			stderr:   "CNI firewall plugin v1.1.0\n",
			expected: "1.1.0",
		},
		{
			stderr:   "CNI firewall plugin v0.8.0\n",
			expected: "0.8.0",
		},
		{
			stderr:   "Foo\nCNI firewall plugin v123.456.789+beta.10\nBar\n",
			expected: "123.456.789+beta.10",
		},
		{
			stderr: "CNI firewall plugin version unknown\n",
			err:    semver.ErrInvalidSemVer.Error(),
		},
		{
			stderr: "",
			err:    "does not have any line that starts with \"CNI firewall plugin \"",
		},
		{
			stderr: "Foo\nBar\nBaz\n",
			err:    "does not have any line that starts with \"CNI firewall plugin \"",
		},
	}

	for _, tc := range testCases {
		got, err := guessFirewallPluginVersion(tc.stderr)
		if tc.err == "" {
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, got.String())
		} else {
			assert.ErrorContains(t, err, tc.err)
		}
	}
}
