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
	"errors"
	"log"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

// TestTest is testing the test tooling itself
func TestTest(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "failure",
			Command:     test.Command("undefinedcommand"),
			Expected:    test.Expects(1, nil, nil),
		},
		{
			Description: "success",
			Command:     test.Command("info"),
			Expected:    test.Expects(0, nil, nil),
		},
		{
			Description: "failure with single error testing",
			Command:     test.Command("undefinedcommand"),
			Expected:    test.Expects(1, []error{errors.New("unknown subcommand")}, nil),
		},
		{
			Description: "success with contains output testing",
			Command:     test.Command("info"),
			Expected:    test.Expects(0, nil, test.Contains("Kernel")),
		},
		{
			Description: "success with negative output testing",
			Command:     test.Command("info"),
			Expected:    test.Expects(0, nil, test.DoesNotContain("foobar")),
		},
		// Note that docker annoyingly returns 125 in a few conditions like this
		{
			Description: "failure with multiple error testing",
			Command:     test.Command("-fail"),
			Expected:    test.Expects(-1, []error{errors.New("unknown"), errors.New("shorthand")}, nil),
		},
		{
			Description: "success with exact output testing",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Custom("echo", "foobar")
			},
			Expected: test.Expects(0, nil, test.Equals("foobar\n")),
		},
		{
			Description: "data propagation",
			Data:        test.WithData("status", "uninitialized"),
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Set("status", data.Get("status")+"-setup")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Custom("printf", data.Get("status"))
				data.Set("status", data.Get("status")+"-command")
				return cmd
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Get("status") == "uninitialized" {
					return
				}
				if data.Get("status") != "uninitialized-setup-command" {
					log.Fatalf("unexpected status label %q", data.Get("status"))
				}
				data.Set("status", data.Get("status")+"-cleanup")
			},
			SubTests: []*test.Case{
				{
					Description: "Subtest data propagation",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Custom("printf", data.Get("status"))
					},
					Expected: test.Expects(0, nil, test.Equals("uninitialized-setup-command")),
				},
			},
			Expected: test.Expects(0, nil, test.Equals("uninitialized-setup")),
		},
	}

	testCase.Run(t)
}
