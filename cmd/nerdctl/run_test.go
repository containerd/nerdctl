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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/ipfs/go-cid"
	httpapi "github.com/ipfs/go-ipfs-http-client"

	"gotest.tools/v3/assert"
)

func TestIPFS(t *testing.T) {
	requiresIPFS(t)
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage)
	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=overlayfs")
	base.Cmd("pull", ipfsCID).AssertOK()
	base.Cmd("run", "--rm", ipfsCID, "echo", "hello").AssertOK()

	// encryption
	keyPair := newJWEKeyPair(t)
	defer keyPair.cleanup()
	encryptImageRef := "newimg:enc"
	layersNum := 1
	base.Cmd("image", "encrypt", "--recipient=jwe:"+keyPair.pub, ipfsCID, encryptImageRef).AssertOK()
	base.Cmd("image", "inspect", "--mode=native", "--format={{len .Manifest.Layers}}", encryptImageRef).AssertOutExactly(fmt.Sprintf("%d\n", layersNum))
	for i := 0; i < layersNum; i++ {
		base.Cmd("image", "inspect", "--mode=native", fmt.Sprintf("--format={{json (index .Manifest.Layers %d) }}", i), encryptImageRef).AssertOutContains("org.opencontainers.image.enc.keys.jwe")
	}
	ipfsCIDEnc := cidOf(t, base.Cmd("push", "ipfs://"+encryptImageRef).OutLines())
	rmiAll(base)

	decryptImageRef := "newimg:dec"
	base.Cmd("pull", "--unpack=false", ipfsCIDEnc).AssertOK()
	base.Cmd("image", "decrypt", "--key="+keyPair.pub, ipfsCIDEnc, decryptImageRef).AssertFail() // decryption needs prv key, not pub key
	base.Cmd("image", "decrypt", "--key="+keyPair.prv, ipfsCIDEnc, decryptImageRef).AssertOK()
	base.Cmd("run", "--rm", decryptImageRef, "/bin/sh", "-c", "echo hello").AssertOK()
}

func TestIPFSCommit(t *testing.T) {
	requiresIPFS(t)
	// cgroup is required for nerdctl commit
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		t.Skip("test skipped for rootless containers on cgroup v1")
	}
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage)

	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=overlayfs")
	base.Cmd("pull", ipfsCID).AssertOK()
	base.Cmd("run", "--rm", ipfsCID, "echo", "hello").AssertOK()
	newContainer, newImg := "hello", "helloimg:v1"
	base.Cmd("run", "--name", "hello", "-d", ipfsCID, "/bin/sh", "-c", "echo hello > /hello ; sleep 10000").AssertOK()
	base.Cmd("commit", newContainer, newImg).AssertOK()
	base.Cmd("stop", newContainer).AssertOK()
	base.Cmd("rm", newContainer).AssertOK()
	ipfsCID2 := cidOf(t, base.Cmd("push", "ipfs://"+newImg).OutLines())
	rmiAll(base)
	base.Cmd("pull", ipfsCID2).AssertOK()
	base.Cmd("run", "--rm", ipfsCID2, "/bin/sh", "-c", "cat /hello").AssertOK()
}

func TestIPFSWithLazyPulling(t *testing.T) {
	requiresIPFS(t)
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage, "--estargz")

	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=stargz")
	base.Cmd("pull", ipfsCID).AssertOK()
	base.Cmd("run", "--rm", ipfsCID, "ls", "/.stargz-snapshotter").AssertOK()
}

func TestIPFSWithLazyPullingCommit(t *testing.T) {
	requiresIPFS(t)
	// cgroup is required for nerdctl commit
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		t.Skip("test skipped for rootless containers on cgroup v1")
	}
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage, "--estargz")

	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=stargz")
	base.Cmd("pull", ipfsCID).AssertOK()
	base.Cmd("run", "--rm", ipfsCID, "ls", "/.stargz-snapshotter").AssertOK()
	newContainer, newImg := "hello", "helloimg:v1"
	base.Cmd("run", "--name", "hello", "-d", ipfsCID, "/bin/sh", "-c", "echo hello > /hello ; sleep 10000").AssertOK()
	base.Cmd("commit", newContainer, newImg).AssertOK()
	base.Cmd("stop", newContainer).AssertOK()
	base.Cmd("rm", newContainer).AssertOK()
	ipfsCID2 := cidOf(t, base.Cmd("push", "--estargz", "ipfs://"+newImg).OutLines())
	rmiAll(base)

	base.Cmd("pull", ipfsCID2).AssertOK()
	base.Cmd("run", "--rm", ipfsCID2, "/bin/sh", "-c", "ls /.stargz-snapshotter && cat /hello").AssertOK()
	base.Cmd("image", "rm", ipfsCID2).AssertOK()
}

func pushImageToIPFS(t *testing.T, base *testutil.Base, name string, opts ...string) string {
	base.Cmd("pull", name).AssertOK()
	ipfsCID := cidOf(t, base.Cmd(append([]string{"push"}, append(opts, "ipfs://"+name)...)...).OutLines())
	base.Cmd("rmi", name).AssertOK()
	return ipfsCID
}

func cidOf(t *testing.T, lines []string) string {
	assert.Equal(t, len(lines) >= 2, true)
	c, err := cid.Decode(lines[len(lines)-2])
	assert.NilError(t, err)
	return "ipfs://" + c.String()
}

func requiresIPFS(t *testing.T) {
	if _, err := httpapi.NewLocalApi(); err != nil {
		t.Skipf("test requires ipfs daemon, but got: %v", err)
	}
	return
}

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
			return fmt.Errorf("expected %q, got %q", expected, stdout)
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
	dir := "/foo"
	if runtime.GOOS == "windows" {
		dir = "c:" + dir
	}
	cmd := base.Cmd("run", "--rm", "--workdir="+dir, testutil.AlpineImage, "pwd")
	cmd.AssertOutContains("/foo")
}

func TestRunWithDoubleDash(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", testutil.AlpineImage, "--", "sh", "-euxc", "exit 0").AssertOK()
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
	tmpDir, err := os.MkdirTemp("", "test-save")
	assert.NilError(base.T, err)
	defer os.RemoveAll(tmpDir)
	archiveTarPath := filepath.Join(tmpDir, "a.tar")
	base.Cmd("save", "-o", archiveTarPath, imageName).AssertOK()
	rootfs, err := os.MkdirTemp("", "rootfs")
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
	base.Cmd("ps", "-a").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "Exited (0)") {
			return fmt.Errorf("no entry for %q", testContainer0)
		}
		if !strings.Contains(stdout, "Exited (123)") {
			return fmt.Errorf("no entry for %q", testContainer123)
		}
		return nil
	})

	inspect0 := base.InspectContainer(testContainer0)
	assert.Equal(base.T, "exited", inspect0.State.Status)
	assert.Equal(base.T, 0, inspect0.State.ExitCode)

	inspect123 := base.InspectContainer(testContainer123)
	assert.Equal(base.T, "exited", inspect123.State.Status)
	assert.Equal(base.T, 123, inspect123.State.ExitCode)
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

func TestRunShmSize(t *testing.T) {
	base := testutil.NewBase(t)
	const shmSize = "32m"

	base.Cmd("run", "--rm", "--shm-size", shmSize, testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts").AssertOutContains("size=32768k")
}

func TestRunEnvFile(t *testing.T) {
	base := testutil.NewBase(t)

	const pattern = "env-file"
	file1, err := os.CreateTemp("", pattern)
	assert.NilError(base.T, err)
	path1 := file1.Name()
	defer file1.Close()
	defer os.Remove(path1)
	err = os.WriteFile(path1, []byte("# this is a comment line\nTESTKEY1=TESTVAL1"), 0666)
	assert.NilError(base.T, err)

	file2, err := os.CreateTemp("", pattern)
	assert.NilError(base.T, err)
	path2 := file2.Name()
	defer file2.Close()
	defer os.Remove(path2)
	err = os.WriteFile(path2, []byte("# this is a comment line\nTESTKEY2=TESTVAL2"), 0666)
	assert.NilError(base.T, err)

	base.Cmd("run", "--rm", "--env-file", path1, "--env-file", path2, testutil.AlpineImage, "sh", "-c", "echo $TESTKEY1").AssertOutContains("TESTVAL1")
	base.Cmd("run", "--rm", "--env-file", path1, "--env-file", path2, testutil.AlpineImage, "sh", "-c", "echo $TESTKEY2").AssertOutContains("TESTVAL2")
}

func TestRunPidHost(t *testing.T) {
	base := testutil.NewBase(t)
	pid := os.Getpid()

	base.Cmd("run", "--rm", "--pid=host", testutil.AlpineImage, "ps", "auxw").AssertOutContains(strconv.Itoa(pid))
}

func TestRunAddHost(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "--add-host", "testing.example.com:10.0.0.1", testutil.AlpineImage, "sh", "-c", "cat /etc/hosts").AssertOutWithFunc(func(stdout string) error {
		var found bool
		sc := bufio.NewScanner(bytes.NewBufferString(stdout))
		for sc.Scan() {
			//removing spaces and tabs separating items
			line := strings.ReplaceAll(sc.Text(), " ", "")
			line = strings.ReplaceAll(line, "\t", "")
			if strings.Contains(line, "10.0.0.1testing.example.com") {
				found = true
			}
		}
		if !found {
			return errors.New("host was not added")
		}
		return nil
	})
	base.Cmd("run", "--rm", "--add-host", "10.0.0.1:testing.example.com", testutil.AlpineImage, "sh", "-c", "cat /etc/hosts").AssertFail()
}

func TestRunUlimit(t *testing.T) {
	base := testutil.NewBase(t)
	ulimit := "nofile=622:622"

	base.Cmd("run", "--rm", "--ulimit", ulimit, testutil.AlpineImage, "sh", "-c", "ulimit -n").AssertOutContains("622")
}

func TestRunEnv(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm",
		"--env", "FOO=foo1,foo2",
		"--env", "BAR=bar1 bar2",
		"--env", "BAZ=",
		"--env", "QUX",
		"--env", "QUUX=quux1",
		"--env", "QUUX=quux2",
		testutil.AlpineImage, "env").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "\nFOO=foo1,foo2\n") {
			return errors.New("got bad FOO")
		}
		if !strings.Contains(stdout, "\nBAR=bar1 bar2\n") {
			return errors.New("got bad BAR")
		}
		if !strings.Contains(stdout, "\nBAZ=\n") && runtime.GOOS != "windows" {
			return errors.New("got bad BAZ")
		}
		if strings.Contains(stdout, "QUX") {
			return errors.New("got bad QUX (should not be set)")
		}
		if !strings.Contains(stdout, "\nQUUX=quux2\n") {
			return errors.New("got bad QUUX")
		}
		return nil
	})
}
