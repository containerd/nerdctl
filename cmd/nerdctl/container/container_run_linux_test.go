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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

func TestRunCustomRootfs(t *testing.T) {
	testCase := nerdtest.Setup()
	// FIXME: root issue is undiagnosed and this is very likely a containerd bug
	// It appears that in certain conditions, the proxy content store info method will fail on the layer of the image
	// Search for func (pcs *proxyContentStore) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	// Note that:
	// - the problem is still here with containerd and nerdctl v2
	// - it seems to affect images that are tagged multiple times, or that share a layer with another image
	// - this test is not parallelized - but the fact that namespacing it solves the problem suggest that something
	// happening in the default namespace BEFORE this test is run is SOMETIMES setting conditions that will make this fail
	// Possible suspects would be concurrent pulls somehow effing things up w. namespaces.
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Write(nerdtest.Namespace, test.ConfigValue(data.Identifier()))
		rootfs := prepareCustomRootfs(data, helpers, testutil.AlpineImage)
		data.Labels().Set("rootfs", rootfs)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("namespace", "remove", data.Identifier())
		if rootfs := data.Labels().Get("rootfs"); rootfs != "" {
			os.RemoveAll(rootfs)
		}
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "cat environ shows PATH",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--rootfs", data.Labels().Get("rootfs"), "/bin/cat", "/proc/self/environ")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")),
		},
		{
			Description: "echo with entrypoint",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--entrypoint", "/bin/echo", "--rootfs", data.Labels().Get("rootfs"), "echo", "foo")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("echo foo\n")),
		},
	}
	testCase.Run(t)
}

func prepareCustomRootfs(data test.Data, h test.Helpers, imageName string) string {
	h.Ensure("pull", "--quiet", imageName)
	tmpDir := data.Temp().Dir("test-save")
	archiveTarPath := filepath.Join(tmpDir, "a.tar")
	h.Ensure("save", "-o", archiveTarPath, imageName)
	rootfs := data.Temp().Dir("rootfs")
	err := helpers.ExtractDockerArchive(archiveTarPath, rootfs)
	assert.NilError(h.T(), err)
	return rootfs
}

func TestRunShmSize(t *testing.T) {
	const shmSize = "32m"
	testCase := nerdtest.Setup()
	testCase.Command = test.Command("run", "--rm", "--shm-size", shmSize, testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("size=32768k"))
	testCase.Run(t)
}

func TestRunShmSizeIPCShareable(t *testing.T) {
	const shmSize = "32m"
	testCase := nerdtest.Setup()
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--name", data.Identifier(), "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts")
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("size=32768k"))
	testCase.Run(t)
}

func TestRunIPCShareableRemoveMount(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier(), "--ipc", "shareable", testutil.AlpineImage, "sleep", "0")
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("rm", data.Identifier())
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)
	testCase.Run(t)
}

func TestRunIPCContainerNotExists(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--name", data.Identifier(), "--ipc", "container:abcd1234", testutil.AlpineImage, "sleep", nerdtest.Infinity)
	}
	testCase.Expected = test.Expects(expect.ExitCodeGenericFail, []error{errors.New("no such container: abcd1234")}, nil)
	testCase.Run(t)
}

func TestRunShmSizeIPCContainer(t *testing.T) {
	const shmSize = "32m"
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier("shared"), "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "sleep", nerdtest.Infinity)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("shared"))
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--ipc=container:"+data.Identifier("shared"), testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts")
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("size=32768k"))
	testCase.Run(t)
}

func TestRunIPCContainer(t *testing.T) {
	const shmSize = "32m"
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier("victim"), "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "sleep", nerdtest.Infinity)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("victim"))
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--ipc=container:"+data.Identifier("victim"), testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts")
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("size=32768k"))
	testCase.Run(t)
}

func TestRunPidHost(t *testing.T) {
	pid := os.Getpid()
	testCase := nerdtest.Setup()
	testCase.Command = test.Command("run", "--rm", "--pid=host", testutil.AlpineImage, "ps", "auxw")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(strconv.Itoa(pid)))
	testCase.Run(t)
}

func TestRunUtsHost(t *testing.T) {
	// Was thinking of os.ReadLink("/proc/1/ns/uts")
	// but you'd get EPERM for rootless. Just validate the
	// hostname is the same.
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostName, err := os.Hostname()
		assert.NilError(helpers.T(), err)
		data.Labels().Set("hostName", hostName)
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "hostname matches host uts",
			Command:     test.Command("run", "--rm", "--uts=host", testutil.AlpineImage, "hostname"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: expect.ExitCodeSuccess, Output: expect.Contains(data.Labels().Get("hostName"))}
			},
		},
		{
			Description: "hostname flag rejected with host uts",
			Command:     test.Command("run", "--rm", "--uts=host", "--hostname=foobar", testutil.AlpineImage, "hostname"),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "domainname flag rejected with host uts",
			Command:     test.Command("run", "--rm", "--uts=host", "--domainname=example.com", testutil.AlpineImage, "hostname"),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}
	testCase.Run(t)
}

func TestRunPidContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier("shared"), testutil.AlpineImage, "sleep", nerdtest.Infinity)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("shared"))
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--pid=container:"+data.Identifier("shared"), testutil.AlpineImage, "ps", "ax")
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("sleep "+nerdtest.Infinity))
	testCase.Run(t)
}

func TestRunIpcHost(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testFilePath := filepath.Join("/dev/shm", fmt.Sprintf("%s-%d", data.Identifier(), os.Geteuid()))
		err := os.WriteFile(testFilePath, []byte(""), 0o644)
		assert.NilError(helpers.T(), err)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		testFilePath := filepath.Join("/dev/shm", fmt.Sprintf("%s-%d", data.Identifier(), os.Geteuid()))
		_ = os.Remove(testFilePath)
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		testFilePath := filepath.Join("/dev/shm", fmt.Sprintf("%s-%d", data.Identifier(), os.Geteuid()))
		return helpers.Command("run", "--rm", "--ipc=host", testutil.AlpineImage, "ls", testFilePath)
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)
	testCase.Run(t)
}

func TestRunAddHost(t *testing.T) {
	// Not parallelizable (https://github.com/containerd/nerdctl/issues/1127)
	response := "This is the expected response for --add-host special IP test."
	const hostPort = 8081
	var server *http.Server
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, response)
		})
		server = &http.Server{Addr: fmt.Sprintf(":%d", hostPort), Handler: mux, ReadTimeout: 30 * time.Second}
		go func() {
			err := server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return
			}
		}()
		var err error
		for i := 0; i < 50; i++ {
			var resp *http.Response
			resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d", hostPort))
			if err == nil {
				_ = resp.Body.Close()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		assert.NilError(helpers.T(), err)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if server == nil {
			return
		}
		err := server.Shutdown(context.Background())
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			assert.NilError(helpers.T(), err)
		}
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "single add-host entry is written",
			NoParallel:  true,
			Command:     test.Command("run", "--rm", "--add-host", "testing.example.com:10.0.0.1", testutil.AlpineImage, "cat", "/etc/hosts"),
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
				assert.NilError(t, assertAddHostEntries(stdout, []string{"10.0.0.1testing.example.com"}))
			}),
		},
		{
			Description: "multiple add-host entries are written",
			NoParallel:  true,
			Command:     test.Command("run", "--rm", "--add-host", "test:10.0.0.1", "--add-host", "test1:10.0.0.1", testutil.AlpineImage, "cat", "/etc/hosts"),
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
				assert.NilError(t, assertAddHostEntries(stdout, []string{"10.0.0.1test", "10.0.0.1test1"}))
			}),
		},
		{
			Description: "invalid add-host input fails",
			NoParallel:  true,
			Command:     test.Command("run", "--rm", "--add-host", "10.0.0.1:testing.example.com", testutil.AlpineImage, "cat", "/etc/hosts"),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "host-gateway resolves host service",
			NoParallel:  true,
			Command:     test.Command("run", "--rm", "--add-host", "test:host-gateway", testutil.NginxAlpineImage, "curl", fmt.Sprintf("test:%d", hostPort)),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals(response)),
		},
	}
	testCase.Run(t)
}

func TestRunAddHostWithCustomHostGatewayIP(t *testing.T) {
	// Not parallelizable (https://github.com/containerd/nerdctl/issues/1127)
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.NoParallel = true
	testCase.Command = test.Command("run", "--rm", "--host-gateway-ip", "192.168.5.2", "--add-host", "test:host-gateway", testutil.AlpineImage, "cat", "/etc/hosts")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
		assert.NilError(t, assertAddHostEntries(stdout, []string{"192.168.5.2test"}))
	})
	testCase.Run(t)
}

func TestRunUlimit(t *testing.T) {
	ulimit := "nofile=622:622"
	ulimit2 := "nofile=622:722"
	testCase := nerdtest.Setup()
	testCase.SubTests = []*test.Case{
		{
			Description: "soft limit matches identical hard limit",
			Command:     test.Command("run", "--rm", "--ulimit", ulimit, testutil.AlpineImage, "sh", "-c", "ulimit -Sn"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("622\n")),
		},
		{
			Description: "hard limit matches identical hard limit",
			Command:     test.Command("run", "--rm", "--ulimit", ulimit, testutil.AlpineImage, "sh", "-c", "ulimit -Hn"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("622\n")),
		},
		{
			Description: "soft limit uses first value",
			Command:     test.Command("run", "--rm", "--ulimit", ulimit2, testutil.AlpineImage, "sh", "-c", "ulimit -Sn"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("622\n")),
		},
		{
			Description: "hard limit uses second value",
			Command:     test.Command("run", "--rm", "--ulimit", ulimit2, testutil.AlpineImage, "sh", "-c", "ulimit -Hn"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("722\n")),
		},
	}
	testCase.Run(t)
}

func TestRunWithInit(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		if _, err := exec.LookPath("tini-custom"); err != nil {
			helpers.T().Skip("required executable doesn't exist in PATH: tini-custom")
		}
	}
	testCase.SubTests = []*test.Case{
		{
			// Unable to handle TERM signal, be killed when timeout
			Description: "without init exits with SIGKILL timeout status",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier("plain"), testutil.AlpineImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("plain"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stop", "--time=3", data.Identifier("plain"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						assert.Equal(t, "137", strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.State.ExitCode}}", data.Identifier("plain"))))
					},
				}
			},
		},
		{
			// Test with --init-binary
			Description: "custom init binary exits with SIGTERM status",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier("custom"), "--init-binary", "tini-custom", testutil.AlpineImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("custom"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stop", "--time=3", data.Identifier("custom"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						assert.Equal(t, "143", strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.State.ExitCode}}", data.Identifier("custom"))))
					},
				}
			},
		},
		{
			// Test with --init
			Description: "default init exits with SIGTERM status",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier("default"), "--init", testutil.AlpineImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("default"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stop", "--time=3", data.Identifier("default"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						assert.Equal(t, "143", strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.State.ExitCode}}", data.Identifier("default"))))
					},
				}
			},
		},
	}
	testCase.Run(t)
}

func TestRunTTY(t *testing.T) {
	const sttyPartialOutput = "speed 38400 baud"

	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "stty with -it",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-it", "--name", data.Identifier(), testutil.CommonImage, "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Contains(sttyPartialOutput)),
		},
		{
			Description: "stty with -t",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-t", "--name", data.Identifier(), testutil.CommonImage, "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Contains(sttyPartialOutput)),
		},
		{
			Description: "stty with -i",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-i", "--name", data.Identifier(), testutil.CommonImage, "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "stty without params",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--name", data.Identifier(), testutil.CommonImage, "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "stty with -td",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-td", "--name", data.Identifier(), testutil.CommonImage, "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, nil),
		},
	}
	testCase.Run(t)
}

func TestRunSigProxy(t *testing.T) {
	testCase := nerdtest.Setup()

	// FIXME: gomodjail signal handling is not working yet: https://github.com/AkihiroSuda/gomodjail/issues/51
	testCase.Require = require.Not(nerdtest.Gomodjail)

	testCase.SubTests = []*test.Case{
		{
			Description: "SigProxyDefault",

			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// FIXME: os.Interrupt will likely not work on Windows
				cmd := nerdtest.RunSigProxyContainer(os.Interrupt, true, nil, data, helpers)
				err := cmd.Signal(os.Interrupt)
				assert.NilError(helpers.T(), err)
				return cmd
			},

			Expected: test.Expects(0, nil, expect.Contains(nerdtest.SignalCaught)),
		},
		{
			Description: "SigProxyTrue",

			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := nerdtest.RunSigProxyContainer(os.Interrupt, true, []string{"--sig-proxy=true"}, data, helpers)
				err := cmd.Signal(os.Interrupt)
				assert.NilError(helpers.T(), err)
				return cmd
			},

			Expected: test.Expects(0, nil, expect.Contains(nerdtest.SignalCaught)),
		},
		{
			Description: "SigProxyFalse",

			// Docker behavior changed sometimes with Docker 27
			// See https://github.com/containerd/nerdctl/issues/4219 for details
			Require: require.Not(nerdtest.Docker),

			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := nerdtest.RunSigProxyContainer(os.Interrupt, true, []string{"--sig-proxy=false"}, data, helpers)
				err := cmd.Signal(os.Interrupt)
				assert.NilError(helpers.T(), err)
				return cmd
			},

			Expected: test.Expects(expect.ExitCodeSignaled, nil, expect.DoesNotContain(nerdtest.SignalCaught)),
		},
	}

	testCase.Run(t)
}

func TestRunWithFluentdLogDriver(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		tempDirectory := data.Temp().Dir("fluentd")
		err := os.Chmod(tempDirectory, 0o777)
		assert.NilError(helpers.T(), err)
		data.Labels().Set("tempDirectory", tempDirectory)
		helpers.Ensure("run", "-d", "--name", data.Identifier("fluentd"), "-p", "24224:24224", "-v", fmt.Sprintf("%s:/fluentd/log", tempDirectory), testutil.FluentdImage)
		time.Sleep(3 * time.Second)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("test"))
		helpers.Anyhow("rm", "-f", data.Identifier("fluentd"))
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "-d", "--log-driver", "fluentd", "--name", data.Identifier("test"), testutil.CommonImage, "sh", "-c", "echo test")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				inspectedContainerID := strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.Id}}", data.Identifier("test")))
				matches, err := filepath.Glob(filepath.Join(data.Labels().Get("tempDirectory"), "data.*.log"))
				assert.NilError(t, err)
				assert.Equal(t, 1, len(matches))
				content, err := os.ReadFile(matches[0])
				assert.NilError(t, err)
				logData := string(content)
				assert.Assert(t, strings.Contains(logData, "test"))
				assert.Assert(t, strings.Contains(logData, inspectedContainerID))
			},
		}
	}
	testCase.Run(t)
}

func TestRunWithFluentdLogDriverWithLogOpt(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		tempDirectory := data.Temp().Dir("fluentd")
		err := os.Chmod(tempDirectory, 0o777)
		assert.NilError(helpers.T(), err)
		data.Labels().Set("tempDirectory", tempDirectory)
		helpers.Ensure("run", "-d", "--name", data.Identifier("fluentd"), "-p", "24225:24224", "-v", fmt.Sprintf("%s:/fluentd/log", tempDirectory), testutil.FluentdImage)
		time.Sleep(3 * time.Second)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("test"))
		helpers.Anyhow("rm", "-f", data.Identifier("fluentd"))
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "-d", "--log-driver", "fluentd", "--log-opt", "fluentd-address=127.0.0.1:24225", "--name", data.Identifier("test"), testutil.CommonImage, "sh", "-c", "echo test2")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				inspectedContainerID := strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.Id}}", data.Identifier("test")))
				matches, err := filepath.Glob(filepath.Join(data.Labels().Get("tempDirectory"), "data.*.log"))
				assert.NilError(t, err)
				assert.Equal(t, 1, len(matches))
				content, err := os.ReadFile(matches[0])
				assert.NilError(t, err)
				logData := string(content)
				assert.Assert(t, strings.Contains(logData, "test2"))
				assert.Assert(t, strings.Contains(logData, inspectedContainerID))
			},
		}
	}
	testCase.Run(t)
}

func TestRunWithOOMScoreAdj(t *testing.T) {
	score := "-42"
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Rootful
	testCase.Command = test.Command("run", "--rm", "--oom-score-adj", score, testutil.AlpineImage, "cat", "/proc/self/oom_score_adj")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(score))
	testCase.Run(t)
}

func TestRunWithDetachKeys(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Run interactively and detach
		cmd := helpers.Command("run", "-it", "--detach-keys=ctrl-a,ctrl-b", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
		cmd.WithFeeder(func() io.Reader {
			// Because of the way we proxy stdin, we have to wait here, otherwise we detach before
			// the rest of the input ever reaches the container
			// Note that this only concerns nerdctl, as docker seems to behave ok LOCALLY.
			// But then, it fails for docker as well ON THE CI. It is unclear why at this point.
			// Arbitrary time pauses would not work: what matters is that the container has started.
			// if !nerdtest.IsDocker() {
			nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			// }
			// ctrl+a and ctrl+b (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
			return bytes.NewReader([]byte{1, 2})
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("detach keys")},
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, t tig.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

func TestRunWithTtyAndDetached(t *testing.T) {
	imageName := testutil.CommonImage
	testCase := nerdtest.Setup()
	testCase.SubTests = []*test.Case{
		{
			// without -t, fail
			Description: "without tty logs not-a-tty error",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier("without-terminal"), imageName, "stty")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier("without-terminal"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Identifier("without-terminal"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Errors:   []error{errors.New("stty: standard input: Not a tty")},
					Output: func(stdout string, t tig.T) {
						assert.Equal(t, "1", strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.State.ExitCode}}", data.Identifier("without-terminal"))))
					},
				}
			},
		},
		{
			// with -t, success
			Description: "with tty logs stty output",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "-t", "--name", data.Identifier("with-terminal"), imageName, "stty")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier("with-terminal"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Identifier("with-terminal"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, "speed 38400 baud; line = 0;"))
						assert.Equal(t, "0", strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.State.ExitCode}}", data.Identifier("with-terminal"))))
					},
				}
			},
		},
	}
	testCase.Run(t)
}

// TestIssue3568 tests https://github.com/containerd/nerdctl/issues/3568
func TestIssue3568(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Description = "Issue #3568 - Detaching from a container started by using --rm option causes the container to be deleted."

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Run interactively and detach
		cmd := helpers.Command("run", "--rm", "-it", "--detach-keys=ctrl-a,ctrl-b", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
		cmd.WithFeeder(func() io.Reader {
			// Because of the way we proxy stdin, we have to wait here, otherwise we detach before
			// the rest of the input ever reaches the container
			// Note that this only concerns nerdctl, as docker seems to behave ok LOCALLY.
			// But then, it fails for docker as well ON THE CI. It is unclear why at this point.
			// Arbitrary time pauses would not work: what matters is that the container has started.
			// if !nerdtest.IsDocker() {
			nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			// }
			// ctrl+a and ctrl+b (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
			return bytes.NewReader([]byte{1, 2})
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("detach keys")},
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, t tig.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

// TestPortBindingWithCustomHost tests https://github.com/containerd/nerdctl/issues/3539
func TestPortBindingWithCustomHost(t *testing.T) {
	testCase := nerdtest.Setup()

	const (
		host     = "127.0.0.2"
		hostPort = 8080
	)
	address := fmt.Sprintf("%s:%d", host, hostPort)

	testCase.SubTests = []*test.Case{
		{
			Description: "Issue #3539 - Access to a container running when 127.0.0.2 is specified in -p in rootless mode.",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(), "-p", fmt.Sprintf("%s:80", address), testutil.NginxAlpineImage)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: expect.All(
						func(stdout string, t tig.T) {
							resp, err := nettestutil.HTTPGet(address, 5, false)
							assert.NilError(t, err)

							respBody, err := io.ReadAll(resp.Body)
							assert.NilError(t, err)
							assert.Assert(t, strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet))
						},
					),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestRunDeviceCDI(t *testing.T) {
	const testCDIVendor1 = `
cdiVersion: "0.3.0"
kind: "vendor1.com/device"
devices:
- name: foo
  containerEdits:
    env:
    - FOO=injected
`
	testCase := nerdtest.Setup()
	// Although CDI injection is supported by Docker, specifying the --cdi-spec-dirs on the command line is not.
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cdiSpecDir := data.Temp().Dir("cdi")
		writeTestCDISpecTigron(helpers.T(), testCDIVendor1, "vendor1.yaml", cdiSpecDir)
		data.Labels().Set("cdiSpecDir", cdiSpecDir)
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("--cdi-spec-dirs", data.Labels().Get("cdiSpecDir"), "run", "--rm", "--device", "vendor1.com/device=foo", testutil.AlpineImage, "env")
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("FOO=injected"))
	testCase.Run(t)
}

func TestRunDeviceCDIWithNerdctlConfig(t *testing.T) {
	const testCDIVendor1 = `
cdiVersion: "0.3.0"
kind: "vendor1.com/device"
devices:
- name: foo
  containerEdits:
    env:
    - FOO=injected
`
	testCase := nerdtest.Setup()
	// Although CDI injection is supported by Docker, specifying the --cdi-spec-dirs on the command line is not.
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cdiSpecDir := data.Temp().Dir("cdi")
		writeTestCDISpecTigron(helpers.T(), testCDIVendor1, "vendor1.yaml", cdiSpecDir)
		tomlPath := filepath.Join(data.Temp().Path(), "nerdctl.toml")
		err := os.WriteFile(tomlPath, []byte(fmt.Sprintf("\ncdi_spec_dirs = [\"%s\"]\n", cdiSpecDir)), 0o400)
		assert.NilError(helpers.T(), err)
		data.Labels().Set("tomlPath", tomlPath)
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command("run", "--rm", "--device", "vendor1.com/device=foo", testutil.AlpineImage, "env")
		cmd.Setenv("NERDCTL_TOML", data.Labels().Get("tomlPath"))
		return cmd
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("FOO=injected"))
	testCase.Run(t)
}

// TestRunGPU tests GPU injection using the --gpus flag.
func TestRunGPU(t *testing.T) {
	const nvidiaSpec = `
cdiVersion: "0.5.0"
kind: "nvidia.com/gpu"
devices:
- name: "0"
  containerEdits:
    env:
    - NVIDIA_GPU_0=injected
- name: "1"
  containerEdits:
    env:
    - NVIDIA_GPU_1=injected
`
	const amdSpec = `
cdiVersion: "0.5.0"
kind: "amd.com/gpu"
devices:
- name: "0"
  containerEdits:
    env:
    - AMD_GPU_0=injected
- name: "1"
  containerEdits:
    env:
    - AMD_GPU_1=injected
`
	const unknownSpec = `
cdiVersion: "0.5.0"
kind: "unknown.com/gpu"
devices:
- name: "0"
  containerEdits:
    env:
    - UNKNOWN_GPU_0=injected
`

	testCases := []runGPUTestCase{
		{
			name:         "nvidia device injection",
			specs:        map[string]string{"nvidia.yaml": nvidiaSpec},
			gpuFlags:     []string{"--gpus", "2"},
			expectedEnvs: []string{"NVIDIA_GPU_0=injected", "NVIDIA_GPU_1=injected"},
		},
		{
			name:         "amd device injection",
			specs:        map[string]string{"amd.yaml": amdSpec},
			gpuFlags:     []string{"--gpus", "2"},
			expectedEnvs: []string{"AMD_GPU_0=injected", "AMD_GPU_1=injected"},
		},
		{
			name:         "multiple vendors",
			specs:        map[string]string{"nvidia.yaml": nvidiaSpec, "amd.yaml": amdSpec},
			gpuFlags:     []string{"--gpus", "1"},
			expectedEnvs: []string{"NVIDIA_GPU_0=injected"},
		},
		{
			name:       "unknown vendor fails",
			specs:      map[string]string{"unknown.yaml": unknownSpec},
			gpuFlags:   []string{"--gpus", "1"},
			expectFail: true,
		},
	}
	testCase := nerdtest.Setup()
	// Although CDI injection is supported by Docker, specifying the --cdi-spec-dirs on the command line is not.
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.SubTests = runGPUCases(testCases)
	testCase.Run(t)
}

// TestRunGPUWithOtherCDIDevices tests GPU CDI injection along with other CDI devices.
func TestRunGPUWithOtherCDIDevices(t *testing.T) {
	const amdSpec = `
cdiVersion: "0.5.0"
kind: "amd.com/gpu"
devices:
- name: "0"
  containerEdits:
    env:
    - AMD_GPU_0=injected
- name: "1"
  containerEdits:
    env:
    - AMD_GPU_1=injected
`
	const vendor1Spec = `
cdiVersion: "0.3.0"
kind: "vendor1.com/device"
devices:
- name: foo
  containerEdits:
    env:
    - FOO=injected
`
	testCase := nerdtest.Setup()
	// Although CDI injection is supported by Docker, specifying the --cdi-spec-dirs on the command line is not.
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cdiSpecDir := data.Temp().Dir("cdi")
		writeTestCDISpecTigron(helpers.T(), amdSpec, "amd.yaml", cdiSpecDir)
		writeTestCDISpecTigron(helpers.T(), vendor1Spec, "vendor1.yaml", cdiSpecDir)
		data.Labels().Set("cdiSpecDir", cdiSpecDir)
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("--cdi-spec-dirs", data.Labels().Get("cdiSpecDir"), "run", "--rm", "--gpus", "2", "--device", "vendor1.com/device=foo", testutil.AlpineImage, "env")
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
		assert.Assert(t, strings.Contains(stdout, "AMD_GPU_0=injected"))
		assert.Assert(t, strings.Contains(stdout, "AMD_GPU_1=injected"))
		assert.Assert(t, strings.Contains(stdout, "FOO=injected"))
	})
	testCase.Run(t)
}

type runGPUTestCase struct {
	name         string
	specs        map[string]string
	gpuFlags     []string
	expectedEnvs []string
	expectFail   bool
}

func runGPUCases(cases []runGPUTestCase) []*test.Case {
	subTests := make([]*test.Case, len(cases))
	for i, tc := range cases {
		i, tc := i, tc
		subTests[i] = &test.Case{
			Description: tc.name,
			Setup: func(data test.Data, helpers test.Helpers) {
				cdiSpecDir := data.Temp().Dir("cdi")
				for fileName, spec := range tc.specs {
					writeTestCDISpecTigron(helpers.T(), spec, fileName, cdiSpecDir)
				}
				data.Labels().Set("cdiSpecDir", cdiSpecDir)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				args := []string{"--cdi-spec-dirs", data.Labels().Get("cdiSpecDir"), "run", "--rm"}
				args = append(args, tc.gpuFlags...)
				args = append(args, testutil.AlpineImage, "env")
				return helpers.Command(args...)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				if tc.expectFail {
					return &test.Expected{ExitCode: expect.ExitCodeGenericFail}
				}
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						for _, expectedEnv := range tc.expectedEnvs {
							assert.Assert(t, strings.Contains(stdout, expectedEnv), expectedEnv+" not found")
						}
					},
				}
			},
		}
	}
	return subTests
}

// assertAddHostEntries checks that each expected entry appears in stdout
// after removing spaces and tabs separating items.
func assertAddHostEntries(stdout string, expected []string) error {
	var found int
	sc := bufio.NewScanner(bytes.NewBufferString(stdout))
	for sc.Scan() {
		line := strings.ReplaceAll(sc.Text(), " ", "")
		line = strings.ReplaceAll(line, "\t", "")
		if strutil.InStringSlice(expected, line) {
			found++
		}
	}
	if found != len(expected) {
		return fmt.Errorf("host was not added, found %d", found)
	}
	return nil
}

func writeTestCDISpecTigron(t tig.T, spec string, fileName string, cdiSpecDir string) {
	err := os.MkdirAll(cdiSpecDir, 0o700)
	assert.NilError(t, err)
	cdiSpecPath := filepath.Join(cdiSpecDir, fileName)
	err = os.WriteFile(cdiSpecPath, []byte(spec), 0o400)
	assert.NilError(t, err)
}

func TestSharedIpcSetup(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Labels().Set("container1", data.Identifier("container1"))
			helpers.Ensure("run", "-d", "--name", data.Identifier("container1"), "--ipc=shareable",
				testutil.CommonImage, "sleep", "inf")
			nerdtest.EnsureContainerStarted(helpers, data.Identifier("container1"))
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier("container1"))
		},
		SubTests: []*test.Case{
			{
				Description: "Test ipc is shared",
				NoParallel:  true, // The validation involves starting of the main container: container1
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier("container2"))
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure(
						"run", "-d", "--name", data.Identifier("container2"),
						"--ipc=container:"+data.Labels().Get("container1"),
						testutil.NginxAlpineImage)
					data.Labels().Set("container2", data.Identifier("container2"))
					nerdtest.EnsureContainerStarted(helpers, data.Identifier("container2"))
				},
				SubTests: []*test.Case{
					{
						NoParallel:  true,
						Description: "Test ipc is shared",
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("exec", data.Labels().Get("container2"), "readlink", "/proc/1/ns/ipc")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								ExitCode: 0,
								Output: expect.All(
									func(stdout string, t tig.T) {
										container1IPC := strings.TrimSpace(helpers.Capture("exec", data.Labels().Get("container1"), "readlink", "/proc/1/ns/ipc"))
										container2IPC := strings.TrimSpace(stdout)
										assert.Equal(t, container1IPC, container2IPC)
									},
								),
							}
						},
					},
					{
						NoParallel:  true,
						Description: "Test ipc is shared after restart",
						Setup: func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("restart", data.Labels().Get("container1"))
							helpers.Ensure("stop", "--time=1", data.Labels().Get("container2"))
							helpers.Ensure("start", data.Labels().Get("container2"))
							nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("container2"))
						},
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("exec", data.Labels().Get("container2"), "readlink", "/proc/1/ns/ipc")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								ExitCode: 0,
								Output: expect.All(
									func(stdout string, t tig.T) {
										container1IPC := strings.TrimSpace(helpers.Capture("exec", data.Labels().Get("container1"), "readlink", "/proc/1/ns/ipc"))
										container2IPC := strings.TrimSpace(stdout)
										assert.Equal(t, container1IPC, container2IPC)
									},
								),
							}
						},
					},
				},
			},
		},
	}
	testCase.Run(t)
}
