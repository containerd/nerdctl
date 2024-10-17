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

package completion

import (
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

func TestCompletion(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: test.Not(nerdtest.Docker),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			helpers.Ensure("network", "create", data.Identifier())
			helpers.Ensure("volume", "create", data.Identifier())
			data.Set("identifier", data.Identifier())
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("network", "rm", data.Identifier())
			helpers.Anyhow("volume", "rm", data.Identifier())
		},
		SubTests: []*test.Case{
			{
				Description: "--cgroup-manager",
				Require:     test.Not(test.Windows),
				Command:     test.Command("__complete", "--cgroup-manager", ""),
				Expected:    test.Expects(0, nil, test.Contains("cgroupfs\n")),
			},
			{
				Description: "--snapshotter",
				Require:     test.Not(test.Windows),
				Command:     test.Command("__complete", "--snapshotter", ""),
				Expected:    test.Expects(0, nil, test.Contains("native\n")),
			},
			{
				Description: "empty",
				Command:     test.Command("__complete", ""),
				Expected:    test.Expects(0, nil, test.Contains("run\t")),
			},
			{
				Description: "build --network",
				Command:     test.Command("__complete", "build", "--network", ""),
				Expected:    test.Expects(0, nil, test.Contains("default\n")),
			},
			{
				Description: "run -",
				Command:     test.Command("__complete", "run", "-"),
				Expected:    test.Expects(0, nil, test.Contains("--network\t")),
			},
			{
				Description: "run --n",
				Command:     test.Command("__complete", "run", "--n"),
				Expected:    test.Expects(0, nil, test.Contains("--network\t")),
			},
			{
				Description: "run --ne",
				Command:     test.Command("__complete", "run", "--ne"),
				Expected:    test.Expects(0, nil, test.Contains("--network\t")),
			},
			{
				Description: "run --net",
				Command:     test.Command("__complete", "run", "--net", ""),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.Contains("host\n"),
							test.Contains(data.Get("identifier")+"\n"),
						),
					}
				},
			},
			{
				Description: "run -it --net",
				Command:     test.Command("__complete", "run", "-it", "--net", ""),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.Contains("host\n"),
							test.Contains(data.Get("identifier")+"\n"),
						),
					}
				},
			},
			{
				Description: "run -ti --rm --net",
				Command:     test.Command("__complete", "run", "-it", "--rm", "--net", ""),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.Contains("host\n"),
							test.Contains(data.Get("identifier")+"\n"),
						),
					}
				},
			},
			{
				Description: "run --restart",
				Command:     test.Command("__complete", "run", "--restart", ""),
				Expected:    test.Expects(0, nil, test.Contains("always\n")),
			},
			{
				Description: "network --rm",
				Command:     test.Command("__complete", "network", "rm", ""),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.DoesNotContain("host\n"),
							test.Contains(data.Get("identifier")+"\n"),
						),
					}
				},
			},
			{
				Description: "run --cap-add",
				Require:     test.Not(test.Windows),
				Command:     test.Command("__complete", "run", "--cap-add", ""),
				Expected: test.Expects(0, nil, test.All(
					test.Contains("sys_admin\n"),
					test.DoesNotContain("CAP_SYS_ADMIN\n"),
				)),
			},
			{
				Description: "volume inspect",
				Command:     test.Command("__complete", "volume", "inspect", ""),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.Contains(data.Get("identifier") + "\n"),
					}
				},
			},
			{
				Description: "volume rm",
				Command:     test.Command("__complete", "volume", "rm", ""),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.Contains(data.Get("identifier") + "\n"),
					}
				},
			},
			{
				Description: "no namespace --cgroup-manager",
				Require:     test.Not(test.Windows),
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Custom("nerdctl", "__complete", "--cgroup-manager", "")
				},
				Expected: test.Expects(0, nil, test.Contains("cgroupfs\n")),
			},
			{
				Description: "no namespace empty",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Custom("nerdctl", "__complete", "")
				},
				Expected: test.Expects(0, nil, test.Contains("run\t")),
			},
			{
				Description: "namespace space empty",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					// mind {"--namespace=nerdctl-test"} vs {"--namespace", "nerdctl-test"}
					return helpers.Custom("nerdctl", "__complete", "--namespace", string(helpers.Read(nerdtest.Namespace)), "")
				},
				Expected: test.Expects(0, nil, test.Contains("run\t")),
			},
			{
				Description: "run -i",
				Command:     test.Command("__complete", "run", "-i", ""),
				Expected:    test.Expects(0, nil, test.Contains(testutil.CommonImage)),
			},
			{
				Description: "run -it",
				Command:     test.Command("__complete", "run", "-it", ""),
				Expected:    test.Expects(0, nil, test.Contains(testutil.CommonImage)),
			},
			{
				Description: "run -it --rm",
				Command:     test.Command("__complete", "run", "-it", "--rm", ""),
				Expected:    test.Expects(0, nil, test.Contains(testutil.CommonImage)),
			},
			{
				Description: "namespace run -i",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					// mind {"--namespace=nerdctl-test"} vs {"--namespace", "nerdctl-test"}
					return helpers.Custom("nerdctl", "__complete", "--namespace", string(helpers.Read(nerdtest.Namespace)), "run", "-i", "")
				},
				Expected: test.Expects(0, nil, test.Contains(testutil.CommonImage+"\n")),
			},
		},
	}

	testCase.Run(t)
}
