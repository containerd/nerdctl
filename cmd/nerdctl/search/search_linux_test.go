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

package search

import (
	"errors"
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// All tests in this file are based on the output of `nerdctl search alpine`.
//
// Expected output format (default behavior with --limit 10):
//
// NAME                DESCRIPTION                                     STARS               OFFICIAL
// alpine              A minimal Docker image based on Alpine Linux…   11437               [OK]
// alpine/git          A  simple git container running in alpine li…   249
// alpine/socat        Run socat command in alpine container           115
// alpine/helm         Auto-trigger docker build for kubernetes hel…   69
// alpine/curl                                                         11
// alpine/k8s          Kubernetes toolbox for EKS (kubectl, helm, i…   64
// alpine/bombardier   Auto-trigger docker build for bombardier whe…   28
// alpine/httpie       Auto-trigger docker build for `httpie` when …   21
// alpine/terragrunt   Auto-trigger docker build for terragrunt whe…   18
// alpine/openssl      openssl                                         7

func TestSearch(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "basic-search",
			Command:     test.Command("search", "alpine", "--limit", "5"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Contains("NAME"),
						expect.Contains("DESCRIPTION"),
						expect.Contains("STARS"),
						expect.Contains("OFFICIAL"),
						expect.Match(regexp.MustCompile(`NAME\s+DESCRIPTION\s+STARS\s+OFFICIAL`)),
						expect.Contains("alpine"),
						expect.Match(regexp.MustCompile(`alpine\s+A minimal Docker image based on Alpine Linux`)),
						expect.Match(regexp.MustCompile(`alpine\s+.*\s+\d+\s+\[OK\]`)),
						expect.Contains("[OK]"),
						expect.Match(regexp.MustCompile(`alpine/\w+`)),
					),
				}
			},
		},
		{
			Description: "search-library-image",
			Command:     test.Command("search", "library/alpine", "--limit", "5"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Contains("NAME"),
						expect.Contains("DESCRIPTION"),
						expect.Contains("STARS"),
						expect.Contains("OFFICIAL"),
						expect.Contains("alpine"),
						expect.Match(regexp.MustCompile(`alpine\s+.*\s+\d+\s+\[OK\]`)),
					),
				}
			},
		},
		{
			Description: "search-with-no-trunc",
			Command:     test.Command("search", "alpine", "--limit", "3", "--no-trunc"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Contains("NAME"),
						expect.Contains("DESCRIPTION"),
						expect.Contains("alpine"),
						// With --no-trunc, the full description should be visible (not truncated with …)
						expect.Match(regexp.MustCompile(`alpine\s+A minimal Docker image based on Alpine Linux with a complete package index and only 5 MB in size!`)),
					),
				}
			},
		},
		{
			Description: "search-with-format",
			Command:     test.Command("search", "alpine", "--limit", "2", "--format", "{{.Name}}: {{.StarCount}}"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Match(regexp.MustCompile(`alpine:\s*\d+`)),
						expect.DoesNotContain("NAME"),
						expect.DoesNotContain("DESCRIPTION"),
						expect.DoesNotContain("OFFICIAL"),
					),
				}
			},
		},
		{
			Description: "search-output-format",
			Command:     test.Command("search", "alpine", "--limit", "5"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Match(regexp.MustCompile(`NAME\s+DESCRIPTION\s+STARS\s+OFFICIAL`)),
						expect.Match(regexp.MustCompile(`(?m)^alpine\s+.*\s+\d+\s+\[OK\]\s*$`)),
						expect.Match(regexp.MustCompile(`(?m)^alpine/\w+\s+.*\s+\d+\s*$`)),
						expect.DoesNotMatch(regexp.MustCompile(`(?m)^\s+\d+\s*$`)),
					),
				}
			},
		},
		{
			Description: "search-description-formatting",
			Command:     test.Command("search", "alpine", "--limit", "10"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Match(regexp.MustCompile(`Alpine Linux…`)),
						expect.DoesNotMatch(regexp.MustCompile(`(?m)^\s+\d+\s+`)),
						expect.Match(regexp.MustCompile(`(?m)^[a-z0-9/_-]+\s+.*\s+\d+`)),
					),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestSearchWithFilter(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "filter-is-official-true",
			Command:     test.Command("search", "alpine", "--filter", "is-official=true", "--limit", "5"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Contains("NAME"),
						expect.Contains("OFFICIAL"),
						expect.Contains("alpine"),
						expect.Contains("[OK]"),
						expect.Match(regexp.MustCompile(`alpine\s+.*\s+\d+\s+\[OK\]`)),
					),
				}
			},
		},
		{
			Description: "filter-stars",
			Command:     test.Command("search", "alpine", "--filter", "stars=10000"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						expect.Contains("NAME"),
						expect.Contains("STARS"),
						expect.Contains("alpine"),
						// The official alpine image has > 10000 stars
						expect.Match(regexp.MustCompile(`alpine\s+.*\s+\d{4,}\s+\[OK\]`)),
					),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestSearchFilterErrors(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "invalid-filter-format",
			Command:     test.Command("search", "alpine", "--filter", "foo"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{errors.New("bad format of filter (expected name=value)")},
				}
			},
		},
		{
			Description: "invalid-filter-key",
			Command:     test.Command("search", "alpine", "--filter", "foo=bar"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{errors.New("invalid filter 'foo'")},
				}
			},
		},
		{
			Description: "invalid-stars-value",
			Command:     test.Command("search", "alpine", "--filter", "stars=abc"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{errors.New("invalid filter 'stars=abc'")},
				}
			},
		},
		{
			Description: "invalid-is-official-value",
			Command:     test.Command("search", "alpine", "--filter", "is-official=abc"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{errors.New("invalid filter 'is-official=abc'")},
				}
			},
		},
	}

	testCase.Run(t)
}
