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

package volume

import (
	"errors"
	"testing"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestVolumeCreate(t *testing.T) {
	nerdtest.Setup()

	tg := &test.Group{
		{
			Description: "arg missing should create anonymous volume",
			Command:     test.RunCommand("volume", "create"),
			Expected:    test.Expects(0, nil, nil),
		},
		{
			Description: "invalid identifier should fail",
			Command:     test.RunCommand("volume", "create", "∞"),
			Expected:    test.Expects(1, []error{errdefs.ErrInvalidArgument}, nil),
		},
		{
			Description: "too many args should fail",
			Command:     test.RunCommand("volume", "create", "too", "many"),
			Expected:    test.Expects(1, []error{errors.New("at most 1 arg")}, nil),
		},
		{
			Description: "success",
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				return helpers.Command("volume", "create", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("volume", "rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Equals(data.Identifier() + "\n"),
				}
			},
		},
		{
			Description: "success with labels",
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				return helpers.Command("volume", "create", "--label", "foo1=baz1", "--label", "foo2=baz2", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("volume", "rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Equals(data.Identifier() + "\n"),
				}
			},
		},
		{
			Description: "invalid labels",
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				// See https://github.com/containerd/nerdctl/issues/3126
				return helpers.Command("volume", "create", "--label", "a", "--label", "", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("volume", "rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					// NOTE: docker returns 125 on this
					ExitCode: -1,
					Errors:   []error{errdefs.ErrInvalidArgument},
				}
			},
		},
		{
			Description: "creating already existing volume should succeed",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("volume", "create", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				return helpers.Command("volume", "create", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("volume", "rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Equals(data.Identifier() + "\n"),
				}
			},
		},
	}

	tg.Run(t)
}
