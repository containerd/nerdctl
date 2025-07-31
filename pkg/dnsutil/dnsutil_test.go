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

package dnsutil

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestValidateIPAddress(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedOut string
		expectedErr string
	}{
		{
			name:        "IPv4 loopback",
			input:       `127.0.0.1`,
			expectedOut: `127.0.0.1`,
		},
		{
			name:        "IPv4 loopback with whitespace",
			input:       ` 127.0.0.1 `,
			expectedOut: `127.0.0.1`,
		},
		{
			name:        "IPv6 loopback long form",
			input:       `0:0:0:0:0:0:0:1`,
			expectedOut: `::1`,
		},
		{
			name:        "IPv6 loopback",
			input:       `::1`,
			expectedOut: `::1`,
		},
		{
			name:        "IPv6 loopback with whitespace",
			input:       ` ::1 `,
			expectedOut: `::1`,
		},
		{
			name:        "IPv6 lowercase",
			input:       `2001:db8::68`,
			expectedOut: `2001:db8::68`,
		},
		{
			name:        "IPv6 uppercase",
			input:       `2001:DB8::68`,
			expectedOut: `2001:db8::68`,
		},
		{
			name:        "IPv6 with brackets",
			input:       `[::1]`,
			expectedErr: `ip address is not correctly formatted: "[::1]"`,
		},
		{
			name:        "IPv4 partial",
			input:       `127`,
			expectedErr: `ip address is not correctly formatted: "127"`,
		},
		{
			name:        "random invalid string",
			input:       `random invalid string`,
			expectedErr: `ip address is not correctly formatted: "random invalid string"`,
		},
		{
			name:        "empty string",
			input:       ``,
			expectedErr: `ip address is not correctly formatted: ""`,
		},
		{
			name:        "only whitespace",
			input:       `   `,
			expectedErr: `ip address is not correctly formatted: "   "`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			actualOut, actualErr := ValidateIPAddress(tc.input)
			assert.Equal(t, tc.expectedOut, actualOut)
			if tc.expectedErr == "" {
				assert.Check(t, actualErr)
			} else {
				assert.Equal(t, tc.expectedErr, actualErr.Error())
			}
		})
	}
}
