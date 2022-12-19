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

package main

import (
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestRunUserGID(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testCases := map[string]string{
		"":       "root bin daemon sys adm disk wheel floppy dialout tape video",
		"1000":   "root",
		"guest":  "users",
		"nobody": "nobody",
	}
	for userStr, expected := range testCases {
		userStr := userStr
		expected := expected
		t.Run(userStr, func(t *testing.T) {
			t.Parallel()
			cmd := []string{"run", "--rm"}
			if userStr != "" {
				cmd = append(cmd, "--user", userStr)
			}
			cmd = append(cmd, testutil.AlpineImage, "id", "-nG")
			base.Cmd(cmd...).AssertOutContains(expected)
		})
	}
}

func TestRunUmask(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testutil.DockerIncompatible(t)
	base.Cmd("run", "--rm", "--umask", "0200", testutil.AlpineImage, "sh", "-c", "umask").AssertOutContains("0200")
}

func TestRunAddGroup(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testCases := []struct {
		user     string
		groups   []string
		expected string
	}{
		{
			user:     "",
			groups:   []string{},
			expected: "root bin daemon sys adm disk wheel floppy dialout tape video",
		},
		{
			user:     "1000",
			groups:   []string{},
			expected: "root",
		},
		{
			user:     "1000",
			groups:   []string{"nogroup"},
			expected: "root nogroup",
		},
		{
			user:     "1000:wheel",
			groups:   []string{"nogroup"},
			expected: "wheel nogroup",
		},
		{
			user:     "root",
			groups:   []string{"nogroup"},
			expected: "root bin daemon sys adm disk wheel floppy dialout tape video nogroup",
		},
		{
			user:     "root:nogroup",
			groups:   []string{"nogroup"},
			expected: "nogroup",
		},
		{
			user:     "guest",
			groups:   []string{"root", "nogroup"},
			expected: "users root nogroup",
		},
		{
			user:     "guest:nogroup",
			groups:   []string{"0"},
			expected: "nogroup root",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.user, func(t *testing.T) {
			t.Parallel()
			cmd := []string{"run", "--rm"}
			if testCase.user != "" {
				cmd = append(cmd, "--user", testCase.user)
			}
			for _, group := range testCase.groups {
				cmd = append(cmd, "--group-add", group)
			}
			cmd = append(cmd, testutil.AlpineImage, "id", "-nG")
			base.Cmd(cmd...).AssertOutExactly(testCase.expected + "\n")
		})
	}
}
