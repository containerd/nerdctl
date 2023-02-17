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

// TestRunAddGroup_CVE_2023_25173 tests https://github.com/advisories/GHSA-hmfx-3pcx-653p
//
// Equates to https://github.com/containerd/containerd/commit/286a01f350a2298b4fdd7e2a0b31c04db3937ea8
func TestRunAddGroup_CVE_2023_25173(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testCases := []struct {
		user     string
		groups   []string
		expected string
	}{
		{
			user:     "",
			groups:   nil,
			expected: "groups=0(root),10(wheel)",
		},
		{
			user:     "",
			groups:   []string{"1", "1234"},
			expected: "groups=0(root),1(daemon),10(wheel),1234",
		},
		{
			user:     "1234",
			groups:   nil,
			expected: "groups=0(root)",
		},
		{
			user:     "1234:1234",
			groups:   nil,
			expected: "groups=1234",
		},
		{
			user:     "1234",
			groups:   []string{"1234"},
			expected: "groups=0(root),1234",
		},
		{
			user:     "daemon",
			groups:   nil,
			expected: "groups=1(daemon)",
		},
		{
			user:     "daemon",
			groups:   []string{"1234"},
			expected: "groups=1(daemon),1234",
		},
	}

	base.Cmd("pull", testutil.BusyboxImage).AssertOK()
	for _, testCase := range testCases {
		cmd := []string{"run", "--rm"}
		if testCase.user != "" {
			cmd = append(cmd, "--user", testCase.user)
		}
		for _, group := range testCase.groups {
			cmd = append(cmd, "--group-add", group)
		}
		cmd = append(cmd, testutil.BusyboxImage, "id")
		base.Cmd(cmd...).AssertOutContains(testCase.expected + "\n")
	}
}
