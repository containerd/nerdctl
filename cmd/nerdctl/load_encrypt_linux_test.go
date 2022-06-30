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
	"path"
	"path/filepath"
	"testing"

	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestLoadEncryptImage(t *testing.T) {
	testutil.DockerIncompatible(t)
	keyPair := newJWEKeyPair(t)
	defer keyPair.cleanup()

	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	encryptImageRef := fmt.Sprintf("%s:encrypted", tID)
	tmpPath := path.Dir(keyPair.pub)
	encryptImageTar := path.Join(tmpPath, fmt.Sprintf("%s.encrypted.tar", tID))
	defer os.Remove(encryptImageTar)

	base.Cmd("pull", testutil.CommonImage).AssertOK()
	base.Cmd("image", "encrypt", "--recipient=jwe:"+keyPair.pub, testutil.CommonImage, encryptImageRef).AssertOK()
	base.Cmd("save", "--output", encryptImageTar, encryptImageRef).AssertOK()
	// remove all local images (in the nerdctl-test namespace), to ensure that we do not have blobs of the original image.
	rmiAll(base)

	base.Cmd("load", "--input", encryptImageTar).AssertFail()
	base.Cmd("load", "--input", encryptImageTar, "--unpack=false").AssertOK()
	base.Cmd("rmi", encryptImageRef).AssertOK()

	// copy temp private key to default directory, from default config.toml --decryption-keys-path value
	ocicryptDefaultPath := "/etc/containerd/ocicrypt/keys"
	if rootlessutil.IsRootless() {
		ocicryptDefaultPath = "~/.config/containerd/ocicrypt/keys"
	}
	err := os.MkdirAll(ocicryptDefaultPath, 0755)
	assert.NilError(t, err)
	if err := exec.Command("cp", "-f", keyPair.prv, ocicryptDefaultPath).Run(); err != nil {
		t.FailNow()
	}
	prvName := filepath.Base(keyPair.prv)
	etcPrvPath := filepath.Join(ocicryptDefaultPath, prvName)
	defer os.Remove(etcPrvPath)

	// decrypt image when private key place in default directory
	base.Cmd("load", "--input", encryptImageTar).AssertOK()
	base.Cmd("run", "--rm", encryptImageRef, "echo", "hello").AssertOutContains("hello")
}
