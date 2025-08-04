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

package manifest

import (
	"errors"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestManifestCreateErrors(t *testing.T) {
	testCase := nerdtest.Setup()
	manifestListName := "test-list:v1"
	manifestName := "example.com/alpine:latest"
	invalidName := "invalid/name/with/special@chars"
	testCase.SubTests = []*test.Case{
		{
			Description: "too-few-arguments",
			Command:     test.Command("manifest", "create", manifestListName),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("error"))},
				}
			},
			Data: test.WithLabels(map[string]string{
				"error": "requires at least 2 arg",
			}),
		},
		{
			Description: "invalid-list-name",
			Command:     test.Command("manifest", "create", invalidName, manifestName),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("error"))},
				}
			},
			Data: test.WithLabels(map[string]string{
				"error": "invalid reference format",
			}),
		},
		{
			Description: "invalid-manifest-reference",
			Command:     test.Command("manifest", "create", manifestListName, invalidName),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("error"))},
				}
			},
			Data: test.WithLabels(map[string]string{
				"error": "invalid reference format",
			}),
		},
	}

	testCase.Run(t)
}

func TestManifestCreate(t *testing.T) {
	testCase := nerdtest.Setup()
	manifestListName := "test-list-create:v1"
	manifestRef := testutil.GetTestImageWithoutTag("alpine") + "@" + testutil.GetTestImageManifestDigest("alpine", "linux/amd64")
	testCase.SubTests = []*test.Case{
		{
			Description: "create-manifest-list",
			Command:     test.Command("manifest", "create", manifestListName, manifestRef),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   expect.Contains(data.Labels().Get("output")),
				}
			},
			Data: test.WithLabels(map[string]string{
				"output": "Created manifest list ",
			}),
		},
		{
			Description: "create-existed-manifest-list-without-amend-flag",
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("manifest", "create", manifestListName+"-without-amend-flag", manifestRef)
				cmd.Run(&test.Expected{ExitCode: 0})
			},
			Command: test.Command("manifest", "create", manifestListName+"-without-amend-flag", manifestRef),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("error"))},
				}
			},
			Data: test.WithLabels(map[string]string{
				"error": "refusing to amend an existing manifest list with no --amend flag",
			}),
		},
		{
			Description: "create-manifest-list-with-amend-flag",
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("manifest", "create", manifestListName+"-with-amend-flag", manifestRef)
				cmd.Run(&test.Expected{ExitCode: 0})
			},
			Command: test.Command("manifest", "create", "--amend", manifestListName+"-with-amend-flag", manifestRef),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   expect.Contains(data.Labels().Get("output")),
				}
			},
			Data: test.WithLabels(map[string]string{
				"output": "Created manifest list",
			}),
		},
	}

	testCase.Run(t)
}
