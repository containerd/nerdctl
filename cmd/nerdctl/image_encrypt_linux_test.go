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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

type jweKeyPair struct {
	prv     string
	pub     string
	cleanup func()
}

func newJWEKeyPair(t testing.TB) *jweKeyPair {
	testutil.RequireExecutable(t, "openssl")
	td, err := os.MkdirTemp(t.TempDir(), "jwe-key-pair")
	assert.NilError(t, err)
	prv := filepath.Join(td, "mykey.pem")
	pub := filepath.Join(td, "mypubkey.pem")
	cmds := [][]string{
		// Exec openssl commands to ensure that nerdctl is compatible with the output of openssl commands.
		// Do NOT refactor this function to use "crypto/rsa" stdlib.
		{"openssl", "genrsa", "-out", prv},
		{"openssl", "rsa", "-in", prv, "-pubout", "-out", pub},
	}
	for _, f := range cmds {
		cmd := exec.Command(f[0], f[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %v (%q)", cmd.Args, err, string(out))
		}
	}
	return &jweKeyPair{
		prv: prv,
		pub: pub,
		cleanup: func() {
			_ = os.RemoveAll(td)
		},
	}
}

func rmiAll(base *testutil.Base) {
	base.T.Logf("Pruning images")
	imageIDs := base.Cmd("images", "--no-trunc", "-a", "-q").OutLines()
	// remove empty output line at the end
	imageIDs = imageIDs[:len(imageIDs)-1]
	// use `Run` on purpose (same below) because `rmi all` may fail on individual
	// image id that has an expected running container (e.g. a registry)
	base.Cmd(append([]string{"rmi", "-f"}, imageIDs...)...).Run()

	base.T.Logf("Pruning build caches")
	if _, err := buildkitutil.GetBuildkitHost(testutil.Namespace); err == nil {
		base.Cmd("builder", "prune").AssertOK()
	}

	// For BuildKit >= 0.11, pruning cache isn't enough to remove manifest blobs that are referred by build history blobs
	// https://github.com/containerd/nerdctl/pull/1833
	if base.Target == testutil.Nerdctl {
		base.T.Logf("Pruning all content blobs")
		addr := base.ContainerdAddress()
		client, err := containerd.New(addr, containerd.WithDefaultNamespace(testutil.Namespace))
		assert.NilError(base.T, err)
		cs := client.ContentStore()
		ctx := context.TODO()
		wf := func(info content.Info) error {
			base.T.Logf("Pruning blob %+v", info)
			if err := cs.Delete(ctx, info.Digest); err != nil {
				base.T.Log(err)
			}
			return nil
		}
		if err := cs.Walk(ctx, wf); err != nil {
			base.T.Log(err)
		}

		base.T.Logf("Pruning all images (again?)")
		imageIDs = base.Cmd("images", "--no-trunc", "-a", "-q").OutLines()
		base.T.Logf("pruning following images: %+v", imageIDs)
		base.Cmd(append([]string{"rmi", "-f"}, imageIDs...)...).Run()
	}
}

func TestImageEncryptJWE(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.DockerIncompatible(t)
	keyPair := newJWEKeyPair(t)
	defer keyPair.cleanup()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	reg := testregistry.NewWithNoAuth(base, 0, false)
	defer reg.Cleanup(nil)
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	encryptImageRef := fmt.Sprintf("127.0.0.1:%d/%s:encrypted", reg.Port, tID)
	defer base.Cmd("rmi", encryptImageRef).Run()
	base.Cmd("image", "encrypt", "--recipient=jwe:"+keyPair.pub, testutil.CommonImage, encryptImageRef).AssertOK()
	base.Cmd("image", "inspect", "--mode=native", "--format={{len .Index.Manifests}}", encryptImageRef).AssertOutExactly("1\n")
	base.Cmd("image", "inspect", "--mode=native", "--format={{json .Manifest.Layers}}", encryptImageRef).AssertOutContains("org.opencontainers.image.enc.keys.jwe")
	base.Cmd("push", encryptImageRef).AssertOK()
	// remove all local images (in the nerdctl-test namespace), to ensure that we do not have blobs of the original image.
	rmiAll(base)
	base.Cmd("pull", encryptImageRef).AssertFail() // defaults to --unpack=true, and fails due to missing prv key
	base.Cmd("pull", "--unpack=false", encryptImageRef).AssertOK()
	decryptImageRef := tID + ":decrypted"
	defer base.Cmd("rmi", decryptImageRef).Run()
	base.Cmd("image", "decrypt", "--key="+keyPair.pub, encryptImageRef, decryptImageRef).AssertFail() // decryption needs prv key, not pub key
	base.Cmd("image", "decrypt", "--key="+keyPair.prv, encryptImageRef, decryptImageRef).AssertOK()
}
