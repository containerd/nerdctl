/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"strconv"
	"strings"
	"testing"

	"github.com/AkihiroSuda/nerdctl/pkg/testutil"
	"github.com/pkg/errors"
)

func TestRunCap(t *testing.T) {
	base := testutil.NewBase(t)
	type testCase struct {
		args   []string
		capEff string
	}
	testCases := []testCase{
		{
			capEff: "00000000a80425fb",
		},
		{
			args:   []string{"--cap-add=all"},
			capEff: "0000003fffffffff",
		},
		{
			args:   []string{"--cap-add=all", "--cap-drop=net_raw"},
			capEff: "0000003fffffdfff",
		},
		{
			args:   []string{"--cap-drop=all", "--cap-add=net_raw"},
			capEff: "0000000000002000",
		},
	}
	for _, tc := range testCases {
		tc := tc // IMPORTANT
		name := "default"
		if len(tc.args) > 0 {
			name = strings.Join(tc.args, "_")
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			args := []string{"run", "--rm"}
			args = append(args, tc.args...)
			args = append(args, testutil.AlpineImage, "grep", "-w", "^CapEff:", "/proc/1/status")
			cmd := base.Cmd(args...)
			cmd.AssertOut(tc.capEff)
		})
	}
}

func TestRunSecurityOptSeccomp(t *testing.T) {
	base := testutil.NewBase(t)
	type testCase struct {
		args    []string
		seccomp int
	}
	testCases := []testCase{
		{
			seccomp: 2,
		},
		{
			args:    []string{"--security-opt", "seccomp=unconfined"},
			seccomp: 0,
		},
		{
			args:    []string{"--privileged"},
			seccomp: 0,
		},
	}
	for _, tc := range testCases {
		tc := tc // IMPORTANT
		name := "default"
		if len(tc.args) > 0 {
			name = strings.Join(tc.args, "_")
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			args := []string{"run", "--rm"}
			args = append(args, tc.args...)
			// NOTE: busybox grep does not support -oP \K
			args = append(args, testutil.AlpineImage, "grep", "-Eo", `^Seccomp:\s*([0-9]+)`, "/proc/1/status")
			cmd := base.Cmd(args...)
			f := func(expectedSeccomp int) func(string) error {
				return func(stdout string) error {
					s := strings.TrimPrefix(stdout, "Seccomp:")
					s = strings.TrimSpace(s)
					i, err := strconv.Atoi(s)
					if err != nil {
						return errors.Wrapf(err, "failed to parse line %q", stdout)
					}
					if i != expectedSeccomp {
						return errors.Errorf("expected Seccomp to be %d, got %d", expectedSeccomp, i)
					}
					return nil
				}
			}
			cmd.AssertOutWithFunc(f(tc.seccomp))
		})
	}
}
