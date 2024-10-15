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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestCopyToContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.RootFul

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		srcFileContent := "test-file-content"
		srcFile := filepath.Join(data.TempDir(), "test-file")
		err := os.WriteFile(srcFile, []byte(srcFileContent), 0o644)
		assert.NilError(t, err)

		data.Set("srcFile", srcFile)
		data.Set("srcUID", strconv.Itoa(os.Geteuid()))
		data.Set("srcFileContent", srcFileContent)
	}

	genSub := func(description string, customSetup func(data test.Data, helpers test.Helpers), stopped bool, success bool) *test.Case {
		tc := &test.Case{
			Description: description,
			Setup: func(data test.Data, helpers test.Helpers) {
				if customSetup != nil {
					customSetup(data, helpers)
				} else {
					helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
				}
				if stopped {
					helpers.Ensure("stop", data.Identifier("container"))
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("container"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Get("srcPath"), data.Identifier("container")+":"+data.Get("destPath"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				exitCode := 0
				if !success {
					exitCode = 1
				}
				return &test.Expected{
					ExitCode: exitCode,
					Output: func(stdout string, info string, t *testing.T) {
						if !success {
							return
						}

						if stopped {
							helpers.Ensure("start", data.Identifier("container"))
						}

						so := helpers.Capture("exec", data.Identifier("container"), "sh", "-c", "--", fmt.Sprintf("cat %q; stat -c %%u %q", data.Get("catPath"), data.Get("catPath")))
						assert.Equal(t, so, fmt.Sprintf("%s%s\n", data.Get("srcFileContent"), data.Get("srcUID")), info)
					},
				}
			},
		}
		return tc
	}

	// For the test matrix, see https://docs.docker.com/engine/reference/commandline/cp/
	testCase.SubTests = []*test.Case{
		{
			Description: "SRC_PATH specifies a file",
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Set("srcPath", data.Get("srcFile"))
			},
			SubTests: []*test.Case{
				{
					Description: "DEST_PATH does not exist",
					Setup: func(data test.Data, helpers test.Helpers) {
						data.Set("destPath", "/dest-no-exist-no-slash")
						data.Set("catPath", "/dest-no-exist-no-slash")
					},
					SubTests: []*test.Case{
						genSub("running", nil, false, true),
						genSub("stopped", nil, true, true),
					},
				},
				{
					Description: "DEST_PATH does not exist and ends with /",
					Setup: func(data test.Data, helpers test.Helpers) {
						data.Set("destPath", "/dest-no-exist-with-slash/")
						data.Set("catPath", "/dest-no-exist-with-slash/")
					},
					SubTests: []*test.Case{
						genSub("running", nil, false, false),
						genSub("stopped", nil, true, false),
					},
				},
				{
					Description: "DEST_PATH exist and is a file",
					Setup: func(data test.Data, helpers test.Helpers) {
						data.Set("destPath", "/dest-file-exists")
						data.Set("catPath", "/dest-file-exists")
					},
					SubTests: []*test.Case{
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "touch", data.Get("destPath"))
						}, false, true),
						genSub("stopped", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "touch", data.Get("destPath"))
						}, true, true),
					},
				},
				{
					Description: "DEST_PATH exist and is a directory",
					Setup: func(data test.Data, helpers test.Helpers) {
						data.Set("destPath", "/dest-dir-exists")
						data.Set("catPath", filepath.Join("/dest-dir-exists", filepath.Base(data.Get("srcPath"))))
					},
					SubTests: []*test.Case{
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "mkdir", "-p", data.Get("destPath"))
						}, false, true),
						genSub("stopped", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "mkdir", "-p", data.Get("destPath"))
						}, true, true),
					},
				},
				{
					Description: "DEST_PATH is the root of a volume",
					Setup: func(data test.Data, helpers test.Helpers) {
						helpers.Ensure("volume", "create", data.Identifier("volume"))
						data.Set("destPath", "/in-a-volume")
						data.Set("catPath", filepath.Join("/in-a-volume", filepath.Base(data.Get("srcPath"))))
					},
					Cleanup: func(data test.Data, helpers test.Helpers) {
						helpers.Anyhow("volume", "rm", data.Identifier("volume"))
					},
					SubTests: []*test.Case{
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), "-v", fmt.Sprintf("%s:%s", data.Identifier("volume"), data.Get("destPath")), testutil.CommonImage, "sleep", "inf")
						}, false, true),
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), "-v", fmt.Sprintf("%s:%s", data.Identifier("volume"), data.Get("destPath")), testutil.CommonImage, "sleep", "inf")
						}, true, true),
					},
				},
				{
					Description: "DEST_PATH is the root of a read-only volume",
					Setup: func(data test.Data, helpers test.Helpers) {
						helpers.Ensure("volume", "create", data.Identifier("volume"))
						data.Set("destPath", "/in-a-volume")
						data.Set("catPath", filepath.Join("/in-a-volume", filepath.Base(data.Get("srcPath"))))
					},
					Cleanup: func(data test.Data, helpers test.Helpers) {
						helpers.Anyhow("volume", "rm", data.Identifier("volume"))
					},
					SubTests: []*test.Case{
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), "-v", fmt.Sprintf("%s:%s:ro", data.Identifier("volume"), data.Get("destPath")), testutil.CommonImage, "sleep", "inf")
						}, false, false),
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), "-v", fmt.Sprintf("%s:%s:ro", data.Identifier("volume"), data.Get("destPath")), testutil.CommonImage, "sleep", "inf")
						}, true, false),
					},
				},
				{
					Description: "DEST_PATH is the root of /tmp (read-only)",
					Setup: func(data test.Data, helpers test.Helpers) {
						helpers.Ensure("volume", "create", data.Identifier("volume"))
						data.Set("destPath", "/tmp")
						data.Set("catPath", filepath.Join("/tmp", filepath.Base(data.Get("srcPath"))))
					},
					Cleanup: func(data test.Data, helpers test.Helpers) {
						helpers.Anyhow("volume", "rm", data.Identifier("volume"))
					},
					SubTests: []*test.Case{
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), "-v", fmt.Sprintf("%s:%s:ro", data.Identifier("volume"), data.Get("destPath")), testutil.CommonImage, "sleep", "inf")
						}, false, false),
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), "-v", fmt.Sprintf("%s:%s:ro", data.Identifier("volume"), data.Get("destPath")), testutil.CommonImage, "sleep", "inf")
						}, true, false),
					},
				},
			},
		},
		{
			Description: "SRC_PATH specifies a directory",
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Set("srcPath", filepath.Dir(data.Get("srcFile")))
			},
			SubTests: []*test.Case{
				{
					Description: "DEST_PATH does not exist",
					Setup: func(data test.Data, helpers test.Helpers) {
						data.Set("destPath", "/dest-no-exist-no-slash")
						data.Set("catPath", filepath.Join("/dest-no-exist-no-slash", filepath.Base(data.Get("srcFile"))))
					},
					SubTests: []*test.Case{
						genSub("running", nil, false, true),
						genSub("stopped", nil, true, true),
					},
				},
				{
					Description: "DEST_PATH exist and is a file",
					Setup: func(data test.Data, helpers test.Helpers) {
						data.Set("destPath", "/dest-file-exists")
					},
					SubTests: []*test.Case{
						genSub("running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "touch", data.Get("destPath"))
						}, false, false),
						genSub("stopped", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "touch", data.Get("destPath"))
						}, true, false),
					},
				},
				{
					Description: "DEST_PATH exist and is a directory",
					Setup: func(data test.Data, helpers test.Helpers) {
						data.Set("destPath", "/dest-dir-exists")
						data.Set("catPath", filepath.Join("/dest-dir-exists", filepath.Base(data.Get("srcPath"))))
					},
					SubTests: []*test.Case{
						genSub("SRC_PATH does not end with `/.` running", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "mkdir", "-p", data.Get("destPath"))
						}, false, true),
						genSub("SRC_PATH does not end with `/.` stopped", func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "mkdir", "-p", data.Get("destPath"))
						}, true, true),
						genSub("SRC_PATH ends with `/.` running", func(data test.Data, helpers test.Helpers) {
							data.Set("srcPath", data.Get("srcPath")+"/.")
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "mkdir", "-p", data.Get("destPath"))
						}, false, true),
						genSub("SRC_PATH ends with `/.` stopped", func(data test.Data, helpers test.Helpers) {
							data.Set("srcPath", data.Get("srcPath")+"/.")
							helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", "inf")
							helpers.Ensure("exec", data.Identifier("container"), "mkdir", "-p", data.Get("destPath"))
						}, true, true),
					},
				},
			},
		},
	}

	testCase.Run(t)
}

func TestCopyFromContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.RootFul

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		srcFileContent := "test-file-content"
		srcFile := filepath.Join("/test-dir", "test-file")

		data.Set("srcFile", srcFile)
		data.Set("srcUID", "42")
		data.Set("eUID", strconv.Itoa(os.Geteuid()))
		data.Set("srcFileContent", srcFileContent)

		helpers.Ensure("run", "-d", "--name", data.Identifier("running"), testutil.CommonImage, "sleep", "inf")
		helpers.Ensure("run", "-d", "--name", data.Identifier("stopped"), testutil.CommonImage, "sleep", "inf")

		mkSrcScript := fmt.Sprintf("mkdir -p %q && echo -n %q >%q && chown %s %q", "/test-dir", srcFileContent, srcFile, data.Get("srcUID"), srcFile)
		helpers.Ensure("exec", data.Identifier("running"), "sh", "-euc", mkSrcScript)
		helpers.Ensure("exec", data.Identifier("stopped"), "sh", "-euc", mkSrcScript)
		helpers.Ensure("stop", data.Identifier("stopped"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("running"))
		helpers.Anyhow("rm", "-f", data.Identifier("stopped"))
	}

	genSub := func(description string, customSetup func(data test.Data, helpers test.Helpers), stopped bool, success bool) *test.Case {
		tc := &test.Case{
			Description: description,
			Setup:       customSetup,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cID := data.Get("running")
				if stopped {
					cID = data.Get("stopped")
				}
				return helpers.Command("cp", cID+":"+data.Get("srcPath"), data.Get("destPath"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				if !success {
					return &test.Expected{
						ExitCode: 1,
					}
				}

				got, err := os.ReadFile(data.Get("catPath"))
				assert.NilError(helpers.T(), err)
				assert.Equal(helpers.T(), data.Get("srcFileContent"), got)
				st, err := os.Stat(data.Get("catPath"))
				assert.NilError(helpers.T(), err)
				stSys := st.Sys().(*syscall.Stat_t)
				// stSys.Uid matches euid, not srcUID
				e, _ := strconv.Atoi(data.Get("eUID"))
				assert.Equal(helpers.T(), uint32(e), stSys.Uid)
				return &test.Expected{}
			},
		}
		return tc
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "SRC_PATH specifies a file",
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Set("srcPath", data.Get("srcFile"))
			},
			SubTests: []*test.Case{
				genSub("DEST_PATH does not exist - running", func(data test.Data, helpers test.Helpers) {
					data.Set("destPath", filepath.Join(data.TempDir(), "dest-no-exist-no-slash"))
					data.Set("catPath", filepath.Join(data.TempDir(), "dest-no-exist-no-slash"))
				}, false, true),
				genSub("DEST_PATH does not exist - stopped", func(data test.Data, helpers test.Helpers) {
					data.Set("destPath", filepath.Join(data.TempDir(), "dest-no-exist-no-slash"))
					data.Set("catPath", filepath.Join(data.TempDir(), "dest-no-exist-no-slash"))
				}, true, true),
				genSub("DEST_PATH does not exist ends / - running", func(data test.Data, helpers test.Helpers) {
					data.Set("destPath", filepath.Join(data.TempDir(), "dest-no-exist-no-slash")+"/")
				}, false, false),
				genSub("DEST_PATH does not exist ends / - stopped", func(data test.Data, helpers test.Helpers) {
					data.Set("destPath", filepath.Join(data.TempDir(), "dest-no-exist-no-slash")+"/")
				}, true, false),
			},
		},
	}

	testCase.Run(t)
}

func TestCopyFromContainerOld(t *testing.T) {
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)
	testStoppedContainer := "stopped-container-" + testutil.Identifier(t)
	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testStoppedContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	defer base.Cmd("rm", "-f", testStoppedContainer).Run()

	euid := os.Geteuid()
	srcUID := 42
	srcDir := "/test-dir"
	srcFile := filepath.Join(srcDir, "test-file")
	srcFileContent := []byte("test-file-content")
	mkSrcScript := fmt.Sprintf("mkdir -p %q && echo -n %q >%q && chown %d %q", srcDir, srcFileContent, srcFile, srcUID, srcFile)
	base.Cmd("exec", testContainer, "sh", "-euc", mkSrcScript).AssertOK()
	base.Cmd("exec", testStoppedContainer, "sh", "-euc", mkSrcScript).AssertOK()
	// Stop container for testing copying out of stopped container
	base.Cmd("stop", testStoppedContainer)

	assertCat := func(catPath string) {
		t.Logf("catPath=%q", catPath)
		got, err := os.ReadFile(catPath)
		assert.NilError(t, err)
		assert.DeepEqual(t, srcFileContent, got)
		st, err := os.Stat(catPath)
		assert.NilError(t, err)
		stSys := st.Sys().(*syscall.Stat_t)
		// stSys.Uid matches euid, not srcUID
		assert.DeepEqual(t, uint32(euid), stSys.Uid)
	}

	td := t.TempDir()
	// For the test matrix, see https://docs.docker.com/engine/reference/commandline/cp/
	t.Run("SRC_PATH specifies a file", func(t *testing.T) {
		srcPath := srcFile
		t.Run("DEST_PATH exists and is a file", func(t *testing.T) {
			destPath := filepath.Join(td, "dest-file-exists")
			err := os.WriteFile(destPath, []byte(""), 0o644)
			assert.NilError(t, err)
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
			catPath := destPath
			assertCat(catPath)
			if rootlessutil.IsRootless() {
				t.Skip("Test skipped in rootless mode for testStoppedContainer")
			}
			base.Cmd("cp", testStoppedContainer+":"+srcPath, destPath).AssertOK()
			assertCat(catPath)
		})
		t.Run("DEST_PATH exists and is a directory", func(t *testing.T) {
			destPath := filepath.Join(td, "dest-dir-exists")
			err := os.Mkdir(destPath, 0o755)
			assert.NilError(t, err)
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
			catPath := filepath.Join(destPath, filepath.Base(srcFile))
			assertCat(catPath)
			if rootlessutil.IsRootless() {
				t.Skip("Test skipped in rootless mode for testStoppedContainer")
			}
			base.Cmd("cp", testStoppedContainer+":"+srcPath, destPath).AssertOK()
			assertCat(catPath)
		})
		t.Run("SRC_PATH is in a volume", func(t *testing.T) {
			// Setup
			// Create a volume
			vol := "somevol"
			base.Cmd("volume", "create", vol).AssertOK()
			defer base.Cmd("volume", "rm", "-f", vol).Run()

			// Create container for test
			con := fmt.Sprintf("%s-with-volume", testContainer)

			mountDir := "/some_dir"
			base.Cmd("run", "-d", "--name", con, "-v", fmt.Sprintf("%s:%s", vol, mountDir), testutil.CommonImage, "sleep", "1h").AssertOK()
			defer base.Cmd("rm", "-f", con).Run()

			// Create a file to mounted volume
			mountedVolFile := filepath.Join(mountDir, "test-file")
			mkSrcScript = fmt.Sprintf("echo -n %q >%q && chown %d %q", srcFileContent, mountedVolFile, srcUID, mountedVolFile)
			base.Cmd("exec", con, "sh", "-euc", mkSrcScript).AssertOK()

			// Create destination directory on host for copy
			destPath := filepath.Join(td, "dest-dir")
			err := os.Mkdir(destPath, 0o700)
			assert.NilError(t, err)

			catPath := filepath.Join(destPath, filepath.Base(mountedVolFile))

			// Running container test
			base.Cmd("cp", con+":"+mountedVolFile, destPath).AssertOK()
			assertCat(catPath)

			// Skip for rootless
			if rootlessutil.IsRootless() {
				t.Skip("Test skipped in rootless mode for testStoppedContainer")
			}
			// Stopped container test
			base.Cmd("stop", con).AssertOK()
			base.Cmd("cp", con+":"+mountedVolFile, destPath).AssertOK()
			assertCat(catPath)
		})
	})
	t.Run("SRC_PATH specifies a directory", func(t *testing.T) {
		srcPath := srcDir
		t.Run("DEST_PATH does not exist", func(t *testing.T) {
			destPath := filepath.Join(td, "dest2-no-exist")
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
			catPath := filepath.Join(destPath, filepath.Base(srcFile))
			assertCat(catPath)
			if rootlessutil.IsRootless() {
				t.Skip("Test skipped in rootless mode for testStoppedContainer")
			}
			base.Cmd("cp", testStoppedContainer+":"+srcPath, destPath).AssertOK()
			assertCat(catPath)
		})
		t.Run("DEST_PATH exists and is a file", func(t *testing.T) {
			destPath := filepath.Join(td, "dest2-file-exists")
			err := os.WriteFile(destPath, []byte(""), 0o644)
			assert.NilError(t, err)
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertFail()
			if rootlessutil.IsRootless() {
				t.Skip("Test skipped in rootless mode for testStoppedContainer")
			}
			base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertFail()
		})
		t.Run("DEST_PATH exists and is a directory", func(t *testing.T) {
			t.Run("SRC_PATH does not end with `/.`", func(t *testing.T) {
				destPath := filepath.Join(td, "dest2-dir-exists")
				err := os.Mkdir(destPath, 0o755)
				assert.NilError(t, err)
				base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
				catPath := filepath.Join(destPath, strings.TrimPrefix(srcFile, filepath.Dir(srcDir)+"/"))
				assertCat(catPath)
				if rootlessutil.IsRootless() {
					t.Skip("Test skipped in rootless mode for testStoppedContainer")
				}
				base.Cmd("cp", testStoppedContainer+":"+srcPath, destPath).AssertOK()
				assertCat(catPath)
			})
			t.Run("SRC_PATH does end with `/.`", func(t *testing.T) {
				srcPath += "/."
				destPath := filepath.Join(td, "dest2-dir2-exists")
				err := os.Mkdir(destPath, 0o755)
				assert.NilError(t, err)
				base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
				catPath := filepath.Join(destPath, filepath.Base(srcFile))
				assertCat(catPath)
				if rootlessutil.IsRootless() {
					t.Skip("Test skipped in rootless mode for testStoppedContainer")
				}
				base.Cmd("cp", testStoppedContainer+":"+srcPath, destPath).AssertOK()
				assertCat(catPath)
			})
		})
	})
}
