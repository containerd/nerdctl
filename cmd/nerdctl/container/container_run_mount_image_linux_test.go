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

package container

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// TestRunMountTypeImage verifies that `--mount type=image` mounts the source
// image's filesystem into the container so its files are readable at the target.
func TestRunMountTypeImage(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img", testutil.CommonImage),
			testutil.CommonImage, "cat", "/mnt/img/etc/os-release")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output:   expect.Contains("Alpine"),
		}
	}

	testCase.Run(t)
}

// TestRunMountTypeImageMultipleDestinations verifies the same image can be
// mounted at two destinations in one container.
func TestRunMountTypeImageMultipleDestinations(t *testing.T) {
	testCase := nerdtest.Setup()
	// nerdctl-only: Docker keys an image mount by its source image and rejects
	// mounting the same image twice ("mount already exists with name").
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/a", testutil.CommonImage),
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/b", testutil.CommonImage),
			testutil.CommonImage, "cat", "/mnt/a/etc/os-release", "/mnt/b/etc/os-release")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output:   expect.Contains("Alpine"),
		}
	}

	testCase.Run(t)
}

// TestRunMountTypeImageReadOnly verifies an image mount is read-only so writing
// fails. This matches Docker, which also mounts images read-only.
func TestRunMountTypeImageReadOnly(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img", testutil.CommonImage),
			testutil.CommonImage, "touch", "/mnt/img/should-fail")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeGenericFail,
			Errors:   []error{fmt.Errorf("Read-only file system")},
		}
	}

	testCase.Run(t)
}

// TestRunMountTypeImageSubpath verifies that image-subpath exposes only the
// selected directory of the image rootfs at the destination: the image's
// /etc/os-release is reachable as <destination>/os-release.
func TestRunMountTypeImageSubpath(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,image-subpath=etc", testutil.CommonImage),
			testutil.CommonImage, "cat", "/mnt/img/os-release")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output:   expect.Contains("Alpine"),
		}
	}

	testCase.Run(t)
}

// TestRunMountTypeImageSubpathMultiple verifies that two image-subpath mounts of
// the same image at different destinations each expose their own subdirectory,
// exercising the multi-mount label round-trip and cleanup.
func TestRunMountTypeImageSubpathMultiple(t *testing.T) {
	testCase := nerdtest.Setup()
	// nerdctl-only: Docker keys an image mount by its source image and rejects
	// mounting the same image twice ("mount already exists with name").
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/etc,image-subpath=etc", testutil.CommonImage),
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/bin,image-subpath=bin", testutil.CommonImage),
			testutil.CommonImage, "ls", "/mnt/etc", "/mnt/bin")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
		}
	}

	testCase.Run(t)
}

// TestRunMountTypeImageSubpathAbsoluteSymlink verifies a subpath through an
// absolute symlink (Alpine's /bin/sh -> /bin/busybox) is rejected, not followed
// against the host. Docker rejects it too but with its own message.
func TestRunMountTypeImageSubpathAbsoluteSymlink(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,image-subpath=bin/sh", testutil.CommonImage),
			testutil.CommonImage, "true")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeGenericFail,
			Errors:   []error{fmt.Errorf("path escapes from parent")},
		}
	}

	testCase.Run(t)
}

// TestRunMountTypeImageSubpathRelativeSymlink verifies a subpath through a
// relative symlink that stays inside the rootfs resolves to the image's own
// target, as it does with Docker.
func TestRunMountTypeImageSubpathRelativeSymlink(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Build

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir -p /data/real /links && echo hello > /data/real/f && ln -s ../data/real /links/rel
`, testutil.CommonImage)
		data.Temp().Save(dockerfile, "Dockerfile")
		helpers.Ensure("build", "-t", data.Identifier("img"), data.Temp().Path())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,image-subpath=links/rel", data.Identifier("img")),
			testutil.CommonImage, "cat", "/mnt/img/f")
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hello\n"))

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", data.Identifier("img"))
	}

	testCase.Run(t)
}

// TestRunMountTypeImageSubpathReadOnly verifies an image-subpath mount is
// read-only so writing fails, matching Docker.
func TestRunMountTypeImageSubpathReadOnly(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,image-subpath=etc", testutil.CommonImage),
			testutil.CommonImage, "touch", "/mnt/img/should-fail")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeGenericFail,
			Errors:   []error{fmt.Errorf("Read-only file system")},
		}
	}

	testCase.Run(t)
}

// TestRunMountTypeImageErrors verifies that an image mount missing its source,
// or using the not-yet-supported subpath option, or an image-subpath that
// escapes the rootfs, is rejected. These are nerdctl-specific behaviours here,
// so the test is not run against Docker.
func TestRunMountTypeImageErrors(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.SubTests = []*test.Case{
		{
			Description: "missing source",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--mount", "type=image,destination=/mnt/img",
					testutil.CommonImage, "true")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{fmt.Errorf("source")},
				}
			},
		},
		{
			Description: "subpath not supported",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,subpath=etc", testutil.CommonImage),
					testutil.CommonImage, "true")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{fmt.Errorf("subpath")},
				}
			},
		},
		{
			Description: "image-subpath parent traversal rejected",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,image-subpath=../etc", testutil.CommonImage),
					testutil.CommonImage, "true")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{fmt.Errorf("escapes")},
				}
			},
		},
		{
			Description: "image-subpath absolute rejected",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,image-subpath=/etc", testutil.CommonImage),
					testutil.CommonImage, "true")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{fmt.Errorf("relative")},
				}
			},
		},
		{
			Description: "empty image-subpath rejected",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=image,source=%s,destination=/mnt/img,image-subpath=", testutil.CommonImage),
					testutil.CommonImage, "true")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{fmt.Errorf("value is empty")},
				}
			},
		},
	}

	testCase.Run(t)
}
