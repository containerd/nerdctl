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

	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestManifestAnnotateErrors(t *testing.T) {
	testCase := nerdtest.Setup()
	manifestListName := "test-list:v1"
	manifestName := "example.com/alpine:latest"
	invalidName := "invalid/name/with/special@chars"
	testCase.SubTests = []*test.Case{
		{
			Description: "too-few-arguments",
			Command:     test.Command("manifest", "annotate", manifestListName),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
				}
			},
		},
		{
			Description: "invalid-list-name",
			Command:     test.Command("manifest", "annotate", invalidName, manifestName),
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
			Command:     test.Command("manifest", "annotate", manifestListName, invalidName),
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

func TestManifestAnnotate(t *testing.T) {
	testCase := nerdtest.Setup()
	manifestListName := "example.com/test-list-annotate:v1"
	manifestRef := testutil.GetTestImageWithoutTag("alpine") + "@" + testutil.GetTestImageManifestDigest("alpine", "linux/amd64")

	testCase.SubTests = []*test.Case{
		{
			Description: "annotate-non-existent-manifest",
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("manifest", "create", manifestListName, manifestRef)
				cmd.Run(&test.Expected{ExitCode: 0})
			},
			Command: test.Command("manifest", "annotate", manifestListName, "example.com/fake:0.0"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("error"))},
				}
			},
			Data: test.WithLabels(map[string]string{
				"error": "manifest for image example.com/fake:0.0 does not exist",
			}),
		},
		{
			Description: "annotate-success",
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("manifest", "create", manifestListName+"-success", manifestRef)
				cmd.Run(&test.Expected{ExitCode: 0})
			},
			Command: test.Command("manifest", "annotate",
				manifestListName+"-success",
				manifestRef,
				"--os", "freebsd",
				"--arch", "arm",
				"--os-version", "1",
				"--os-features", "feature1",
				"--variant", "v7"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
				}
			},
		},
	}

	testCase.Run(t)
}
