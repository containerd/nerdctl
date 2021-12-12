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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestCopyToContainer(t *testing.T) {
	tID := testutil.Identifier(t)
	if runtime.GOOS == "windows" {
		t.Skipf("copying to container doesn't support windows platform yet, skip test %s", tID)
	}

	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()

	workDir, workFile, err := createWorkDirWith("workFile")
	assert.NilError(t, err)
	defer os.RemoveAll(workDir)

	workFilePath := filepath.Join(workDir, workFile)
	t.Logf("copying %s from host to %s", workFilePath, fmt.Sprintf("%s:%s", testContainer, "/tmp"))

	base.Cmd("cp", workFilePath, fmt.Sprintf("%s:%s", testContainer, "/tmp")).AssertOK()

	base.Cmd("exec", testContainer, "cat", filepath.Join("/tmp", workFile)).AssertOutExactly("success")
	base.Cmd("rm", "-f", testContainer).Run()
}

func TestCopyFromContainer(t *testing.T) {
	tID := testutil.Identifier(t)
	if runtime.GOOS == "windows" {
		t.Skipf("copying from container doesn't support windows platform yet, skip test %s", tID)
	}

	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()

	workDir, _, err := createWorkDirWith("")
	assert.NilError(t, err)

	defer os.RemoveAll(workDir)

	etcOSReleasePath := filepath.Join(workDir, "os-release")

	t.Logf("copying %s from %s to %s in the host", "/etc/os-release", testContainer, etcOSReleasePath)

	base.Cmd("cp", fmt.Sprintf("%s:%s", testContainer, "/etc/os-release"), etcOSReleasePath).AssertOK()

	etcOSReleaseBytes, err := os.ReadFile(etcOSReleasePath)
	assert.NilError(t, err)

	etcOSRelease := string(etcOSReleaseBytes)

	t.Log(etcOSRelease)
	assert.Assert(t, strings.Contains(etcOSRelease, "Alpine"))
	base.Cmd("rm", "-f", testContainer).Run()
}

func createWorkDirWith(file string) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "nerdctl-cp-test")
	if err != nil {
		return "", "", err
	}
	if file != "" {
		if err = os.WriteFile(filepath.Join(tmpDir, file), []byte("success"), 0777); err != nil {
			return "", "", err
		}
	}
	return tmpDir, file, nil
}
