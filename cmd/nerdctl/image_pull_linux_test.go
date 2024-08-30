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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

type cosignKeyPair struct {
	publicKey  string
	privateKey string
	cleanup    func()
}

func newCosignKeyPair(t testing.TB, path string, password string) *cosignKeyPair {
	td, err := os.MkdirTemp(t.TempDir(), path)
	assert.NilError(t, err)

	cmd := exec.Command("cosign", "generate-key-pair")
	cmd.Dir = td
	cmd.Env = append(cmd.Env, fmt.Sprintf("COSIGN_PASSWORD=%s", password))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to run %v: %v (%q)", cmd.Args, err, string(out))
	}

	publicKey := filepath.Join(td, "cosign.pub")
	privateKey := filepath.Join(td, "cosign.key")

	return &cosignKeyPair{
		publicKey:  publicKey,
		privateKey: privateKey,
		cleanup: func() {
			_ = os.RemoveAll(td)
		},
	}
}

func TestImageVerifyWithCosign(t *testing.T) {
	testutil.RequireExecutable(t, "cosign")
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	base := testutil.NewBase(t)
	base.Env = append(base.Env, "COSIGN_PASSWORD=1")
	keyPair := newCosignKeyPair(t, "cosign-key-pair", "1")
	defer keyPair.cleanup()
	tID := testutil.Identifier(t)
	reg := testregistry.NewWithNoAuth(base, 0, false)
	defer reg.Cleanup(nil)
	localhostIP := "127.0.0.1"
	t.Logf("localhost IP=%q", localhostIP)
	testImageRef := fmt.Sprintf("%s:%d/%s",
		localhostIP, reg.Port, tID)
	t.Logf("testImageRef=%q", testImageRef)

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)

	base.Cmd("build", "-t", testImageRef, buildCtx).AssertOK()
	base.Cmd("push", testImageRef, "--sign=cosign", "--cosign-key="+keyPair.privateKey).AssertOK()
	base.Cmd("pull", testImageRef, "--verify=cosign", "--cosign-key="+keyPair.publicKey).AssertOK()
}

func TestImagePullPlainHttpWithDefaultPort(t *testing.T) {
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	base := testutil.NewBase(t)
	reg := testregistry.NewWithNoAuth(base, 80, false)
	defer reg.Cleanup(nil)
	testImageRef := fmt.Sprintf("%s/%s:%s",
		reg.IP.String(), testutil.Identifier(t), strings.Split(testutil.CommonImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	t.Logf("testImageRef=%q", testImageRef)
	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)
	base.Cmd("build", "-t", testImageRef, buildCtx).AssertOK()
	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
	base.Cmd("--insecure-registry", "pull", testImageRef).AssertOK()
}

func TestImageVerifyWithCosignShouldFailWhenKeyIsNotCorrect(t *testing.T) {
	testutil.RequireExecutable(t, "cosign")
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	base := testutil.NewBase(t)
	base.Env = append(base.Env, "COSIGN_PASSWORD=1")
	keyPair := newCosignKeyPair(t, "cosign-key-pair", "1")
	defer keyPair.cleanup()
	tID := testutil.Identifier(t)
	reg := testregistry.NewWithNoAuth(base, 0, false)
	defer reg.Cleanup(nil)
	localhostIP := "127.0.0.1"
	t.Logf("localhost IP=%q", localhostIP)
	testImageRef := fmt.Sprintf("%s:%d/%s",
		localhostIP, reg.Port, tID)
	t.Logf("testImageRef=%q", testImageRef)

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)

	base.Cmd("build", "-t", testImageRef, buildCtx).AssertOK()
	base.Cmd("push", testImageRef, "--sign=cosign", "--cosign-key="+keyPair.privateKey).AssertOK()
	base.Cmd("pull", testImageRef, "--verify=cosign", "--cosign-key="+keyPair.publicKey).AssertOK()

	base.Env = append(base.Env, "COSIGN_PASSWORD=2")
	newKeyPair := newCosignKeyPair(t, "cosign-key-pair-test", "2")
	base.Cmd("pull", testImageRef, "--verify=cosign", "--cosign-key="+newKeyPair.publicKey).AssertFail()
}

func TestPullSoci(t *testing.T) {
	testutil.DockerIncompatible(t)
	tests := []struct {
		name                         string
		sociIndexDigest              string
		image                        string
		remoteSnapshotsExpectedCount int
	}{
		{
			name:                         "Run without specifying SOCI index",
			sociIndexDigest:              "",
			image:                        testutil.FfmpegSociImage,
			remoteSnapshotsExpectedCount: 11,
		},
		{
			name:                         "Run with bad SOCI index",
			sociIndexDigest:              "sha256:thisisabadindex0000000000000000000000000000000000000000000000000",
			image:                        testutil.FfmpegSociImage,
			remoteSnapshotsExpectedCount: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := testutil.NewBase(t)
			requiresSoci(base)

			//counting initial snapshot mounts
			initialMounts, err := exec.Command("mount").Output()
			if err != nil {
				t.Fatal(err)
			}

			remoteSnapshotsInitialCount := strings.Count(string(initialMounts), "fuse.rawBridge")

			pullOutput := base.Cmd("--snapshotter=soci", "pull", tt.image).Out()
			base.T.Logf("pull output: %s", pullOutput)

			actualMounts, err := exec.Command("mount").Output()
			if err != nil {
				t.Fatal(err)
			}
			remoteSnapshotsActualCount := strings.Count(string(actualMounts), "fuse.rawBridge")
			base.T.Logf("number of actual mounts: %v", remoteSnapshotsActualCount-remoteSnapshotsInitialCount)

			rmiOutput := base.Cmd("rmi", testutil.FfmpegSociImage).Out()
			base.T.Logf("rmi output: %s", rmiOutput)

			base.T.Logf("number of expected mounts: %v", tt.remoteSnapshotsExpectedCount)

			if tt.remoteSnapshotsExpectedCount != (remoteSnapshotsActualCount - remoteSnapshotsInitialCount) {
				t.Fatalf("incorrect number of remote snapshots; expected=%d, actual=%d",
					tt.remoteSnapshotsExpectedCount, remoteSnapshotsActualCount-remoteSnapshotsInitialCount)
			}
		})
	}
}
