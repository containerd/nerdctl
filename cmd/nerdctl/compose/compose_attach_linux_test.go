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

package compose

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeAttach(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Like nerdctl attach, compose attach needs a container created with stdin open and a TTY.
		dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
    stdin_open: true
    tty: true
`, testutil.CommonImage)

		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		data.Labels().Set("composeYAML", composePath)

		// compose up runs a service with stdin_open and tty like nerdctl run -it, in the foreground:
		// give it a console and detach from it, like TestAttach does with nerdctl run -it.
		cmd := helpers.Command("compose", "-f", composePath, "up", "-d")
		cmd.WithPseudoTTY()
		// ctrl+p and ctrl+q (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
		cmd.Feed(bytes.NewReader([]byte{16, 17}))
		cmd.Run(&test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Errors:   []error{errors.New("read detach keys")},
		})
		nerdtest.EnsureContainerStarted(helpers, serviceparser.DefaultContainerName(projectName, "svc0", "1"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "rejects an index out of range",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "attach", "--index", "2", "svc0")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "attaches to the service container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "attach", "svc0")
				cmd.WithPseudoTTY()
				cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
				cmd.WithFeeder(func() io.Reader {
					// Give the container time to read stdin and answer before detaching. compose attach
					// loads the project and runs nerdctl attach in a subprocess first, so this takes
					// longer than with nerdctl attach.
					time.Sleep(3 * time.Second)
					// ctrl+p and ctrl+q (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
					return bytes.NewReader([]byte{16, 17})
				})
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					// Detaching from attach exits with status 1, like docker.
					// https://github.com/containerd/nerdctl/issues/3571
					ExitCode: 1,
					Errors:   []error{errors.New("read detach keys")},
					Output:   expect.Contains("markmark"),
				}
			},
		},
	}

	testCase.Run(t)
}
