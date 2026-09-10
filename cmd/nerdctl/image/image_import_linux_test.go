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

package image

import (
	"archive/tar"
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// minimalRootfsTar returns a valid tar archive with no files.
func minimalRootfsTar(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	assert.NilError(t, tw.Close())
	return buf
}

// minimalImageArchiveTar returns a tar that looks like a standard image archive
// (it carries a manifest.json), used to exercise the --change rejection path.
func minimalImageArchiveTar(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	content := []byte("[]")
	assert.NilError(t, tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(content)), Mode: 0644}))
	_, err := tw.Write(content)
	assert.NilError(t, err)
	assert.NilError(t, tw.Close())
	return buf
}

func TestImageImportErrors(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestImageImportErrors",
		Require:     require.Linux,
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("import", "", "image:tag")
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1,
				Errors:   []error{errors.New(data.Labels().Get("error"))},
			}
		},
		Data: test.WithLabels(map[string]string{
			"error": "no such file or directory",
		}),
	}

	testCase.Run(t)
}

func TestImageImport(t *testing.T) {
	testCase := nerdtest.Setup()

	var stopServer func()

	testCase.SubTests = []*test.Case{
		{
			Description: "image import from stdin",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("import", "-", data.Identifier())
				cmd.Feed(bytes.NewReader(minimalRootfsTar(t).Bytes()))
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				identifier := data.Identifier()
				return &test.Expected{
					Output: expect.All(
						func(stdout string, t tig.T) {
							imgs := helpers.Capture("images")
							assert.Assert(t, strings.Contains(imgs, identifier))
						},
					),
				}
			},
		},
		{
			Description: "image import from file",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				p := filepath.Join(data.Temp().Path(), "rootfs.tar")
				assert.NilError(t, os.WriteFile(p, minimalRootfsTar(t).Bytes(), 0644))
				data.Labels().Set("tar", p)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("import", data.Labels().Get("tar"), data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				identifier := data.Identifier()
				return &test.Expected{
					Output: expect.All(
						func(stdout string, t tig.T) {
							imgs := helpers.Capture("images")
							assert.Assert(t, strings.Contains(imgs, identifier))
						},
					),
				}
			},
		},
		{
			Description: "image import with message",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("import", "-m", "A message", "-", data.Identifier())
				cmd.Feed(bytes.NewReader(minimalRootfsTar(t).Bytes()))
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				identifier := data.Identifier() + ":latest"
				return &test.Expected{
					Output: expect.All(
						func(stdout string, t tig.T) {
							img := nerdtest.InspectImage(helpers, identifier)
							assert.Equal(t, img.Comment, "A message")
						},
					),
				}
			},
		},
		{
			Description: "image import with change",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("import",
					"--change", `CMD ["echo","hi"]`,
					"--change", "ENV FOO=bar",
					"--change", "WORKDIR /srv",
					"--change", "EXPOSE 8080",
					"-", data.Identifier())
				cmd.Feed(bytes.NewReader(minimalRootfsTar(t).Bytes()))
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				identifier := data.Identifier() + ":latest"
				return &test.Expected{
					Output: expect.All(
						func(stdout string, t tig.T) {
							img := nerdtest.InspectImage(helpers, identifier)
							assert.Assert(t, img.Config != nil)
							assert.DeepEqual(t, img.Config.Cmd, []string{"echo", "hi"})
							assert.Assert(t, slices.Contains(img.Config.Env, "FOO=bar"))
							assert.Equal(t, img.Config.WorkingDir, "/srv")
							_, ok := img.Config.ExposedPorts["8080/tcp"]
							assert.Assert(t, ok)
						},
					),
				}
			},
		},
		{
			Description: "image import --change rejected for a standard image archive",
			// nerdctl-only: Docker's import treats any tarball as a rootfs and has
			// no standard-image-archive rejection.
			Require: require.Not(nerdtest.Docker),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("import", "--change", `CMD ["echo"]`, "-", data.Identifier())
				cmd.Feed(bytes.NewReader(minimalImageArchiveTar(t).Bytes()))
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Errors:   []error{errors.New("filesystem archive")},
				}
			},
		},
		{
			Description: "image import with platform",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("import", "--platform", "linux/amd64", "-", data.Identifier())
				cmd.Feed(bytes.NewReader(minimalRootfsTar(t).Bytes()))
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				identifier := data.Identifier() + ":latest"
				return &test.Expected{
					Output: expect.All(
						func(stdout string, t tig.T) {
							img := nerdtest.InspectImage(helpers, identifier)
							assert.Equal(t, img.Architecture, "amd64")
							assert.Equal(t, img.Os, "linux")
						},
					),
				}
			},
		},
		{
			Description: "image import from URL",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if stopServer != nil {
					stopServer()
					stopServer = nil
				}
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/x-tar")
					_, _ = w.Write(minimalRootfsTar(t).Bytes())
				})
				url, stop, err := nerdtest.StartHTTPServer(handler)
				assert.NilError(t, err)
				stopServer = stop
				data.Labels().Set("url", url)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("import", data.Labels().Get("url"), data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				identifier := data.Identifier()
				return &test.Expected{
					Output: expect.All(
						func(stdout string, t tig.T) {
							imgs := helpers.Capture("images")
							assert.Assert(t, strings.Contains(imgs, identifier))
						},
					),
				}
			},
		},
	}
	testCase.Run(t)
}
