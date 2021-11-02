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
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

type jweKeyPair struct {
	prv     string
	pub     string
	cleanup func()
}

func newJWEKeyPair(t testing.TB) *jweKeyPair {
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
	imageIDs := base.Cmd("images", "--no-trunc", "-a", "-q").OutLines()
	base.Cmd(append([]string{"rmi", "-f"}, imageIDs...)...).AssertOK()
}

func TestImageEncryptJWE(t *testing.T) {
	testutil.DockerIncompatible(t)
	keyPair := newJWEKeyPair(t)
	defer keyPair.cleanup()
	base := testutil.NewBase(t)
	reg := newTestRegistry(base, "test-image-encrypt-jwe")
	defer reg.cleanup()
	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	encryptImageRef := fmt.Sprintf("127.0.0.1:%d/test-image-encrypt-jwe:encrypted", reg.listenPort)
	defer base.Cmd("rmi", encryptImageRef).Run()
	base.Cmd("image", "encrypt", "--recipient=jwe:"+keyPair.pub, testutil.AlpineImage, encryptImageRef).AssertOK()
	base.Cmd("image", "inspect", "--mode=native", "--format={{len .Index.Manifests}}", encryptImageRef).AssertOutExactly("1\n")
	base.Cmd("image", "inspect", "--mode=native", "--format={{json .Manifest.Layers}}", encryptImageRef).AssertOutContains("org.opencontainers.image.enc.keys.jwe")
	base.Cmd("push", encryptImageRef).AssertOK()
	// remove all local images (in the nerdctl-test namespace), to ensure that we do not have blobs of the original image.
	rmiAll(base)
	base.Cmd("pull", encryptImageRef).AssertFail() // defaults to --unpack=true, and fails due to missing prv key
	base.Cmd("pull", "--unpack=false", encryptImageRef).AssertOK()
	decryptImageRef := "test-image-encrypt-jwe:decrypted"
	defer base.Cmd("rmi", decryptImageRef).Run()
	base.Cmd("image", "decrypt", "--key="+keyPair.pub, encryptImageRef, decryptImageRef).AssertFail() // decryption needs prv key, not pub key
	base.Cmd("image", "decrypt", "--key="+keyPair.prv, encryptImageRef, decryptImageRef).AssertOK()
}
