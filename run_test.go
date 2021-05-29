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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
)

func TestRunEntrypointWithBuild(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const imageName = "nerdctl-test-entrypoint-with-build"
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
ENTRYPOINT ["echo", "foo"]
CMD ["echo", "bar"]
	`, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutWithFunc(func(stdout string) error {
		expected := "foo echo bar\n"
		if stdout != expected {
			return errors.Errorf("expected %q, got %q", expected, stdout)
		}
		return nil
	})
	base.Cmd("run", "--rm", "--entrypoint", "", imageName).AssertFail()
	base.Cmd("run", "--rm", "--entrypoint", "", imageName, "echo", "blah").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "blah") {
			return errors.New("echo blah was not executed?")
		}
		if strings.Contains(stdout, "bar") {
			return errors.New("echo bar should not be executed")
		}
		if strings.Contains(stdout, "foo") {
			return errors.New("echo foo should not be executed")
		}
		return nil
	})
	base.Cmd("run", "--rm", "--entrypoint", "time", imageName).AssertFail()
	base.Cmd("run", "--rm", "--entrypoint", "time", imageName, "echo", "blah").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "blah") {
			return errors.New("echo blah was not executed?")
		}
		if strings.Contains(stdout, "bar") {
			return errors.New("echo bar should not be executed")
		}
		if strings.Contains(stdout, "foo") {
			return errors.New("echo foo should not be executed")
		}
		return nil
	})
}

func TestRunWorkdir(t *testing.T) {
	base := testutil.NewBase(t)
	cmd := base.Cmd("run", "--rm", "--workdir=/foo", testutil.AlpineImage, "pwd")
	cmd.AssertOutContains("/foo")
}

func TestRunCustomRootfs(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	rootfs := prepareCustomRootfs(base, testutil.AlpineImage)
	defer os.RemoveAll(rootfs)
	base.Cmd("run", "--rm", "--rootfs", rootfs, "/bin/cat", "/proc/self/environ").AssertOutContains("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	base.Cmd("run", "--rm", "--entrypoint", "/bin/echo", "--rootfs", rootfs, "echo", "foo").AssertOutContains("echo foo")
}

func prepareCustomRootfs(base *testutil.Base, imageName string) string {
	base.Cmd("pull", imageName).AssertOK()
	tmpDir, err := ioutil.TempDir("", "test-save")
	assert.NilError(base.T, err)
	defer os.RemoveAll(tmpDir)
	archiveTarPath := filepath.Join(tmpDir, "a.tar")
	base.Cmd("save", "-o", archiveTarPath, imageName).AssertOK()
	rootfs, err := ioutil.TempDir("", "rootfs")
	assert.NilError(base.T, err)
	err = extractDockerArchive(archiveTarPath, rootfs)
	assert.NilError(base.T, err)
	return rootfs
}

func TestRunExitCode(t *testing.T) {
	base := testutil.NewBase(t)
	const (
		testContainer0   = "nerdctl-test-run-exit-code-0"
		testContainer123 = "nerdctl-test-run-exit-code-123"
	)
	defer base.Cmd("rm", "-f", testContainer0, testContainer123).Run()

	base.Cmd("run", "--name", testContainer0, testutil.AlpineImage, "sh", "-euxc", "exit 0").AssertOK()
	base.Cmd("run", "--name", testContainer123, testutil.AlpineImage, "sh", "-euxc", "exit 123").AssertExitCode(123)
}

func TestRunStatus(t *testing.T) {
	base := testutil.NewBase(t)
	const (
		testContainerDetach123 = "nerdctl-test-run-exit-code-detach-123"
	)
	defer base.Cmd("rm", "-f", testContainerDetach123).Run()

	base.Cmd("run", "-d", "--name", testContainerDetach123, testutil.AlpineImage, "sh", "-euxc", "sleep 5; exit 123").AssertExitCode(0)

	inspect := base.InspectContainer(testContainerDetach123)
	assert.Equal(base.T, "running", inspect.State.Status)

	// check wait status
	const expectedWaitResponse = "123"
	base.Cmd("wait", testContainerDetach123).AssertOutContains(expectedWaitResponse)
}

func TestRunCIDFile(t *testing.T) {
	base := testutil.NewBase(t)
	const fileName = "cid.file"

	base.Cmd("run", "--rm", "--cidfile", fileName, testutil.AlpineImage).AssertOK()
	defer os.Remove(fileName)

	_, err := os.Stat(fileName)
	assert.NilError(base.T, err)

	base.Cmd("run", "--rm", "--cidfile", fileName, testutil.AlpineImage).AssertFail()
}
