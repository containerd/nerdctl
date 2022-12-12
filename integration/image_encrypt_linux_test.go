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

package integration

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
)

func TestImageEncryptJWE(t *testing.T) {
	testutil.DockerIncompatible(t)
	keyPair := utils.NewJWEKeyPair(t)
	defer keyPair.Cleanup()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	reg := testregistry.NewPlainHTTP(base, 5000)
	defer reg.Cleanup()
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	encryptImageRef := fmt.Sprintf("127.0.0.1:%d/%s:encrypted", reg.ListenPort, tID)
	defer base.Cmd("rmi", encryptImageRef).Run()
	base.Cmd("image", "encrypt", "--recipient=jwe:"+keyPair.Pub, testutil.CommonImage, encryptImageRef).AssertOK()
	base.Cmd("image", "inspect", "--mode=native", "--format={{len .Index.Manifests}}", encryptImageRef).AssertOutExactly("1\n")
	base.Cmd("image", "inspect", "--mode=native", "--format={{json .Manifest.Layers}}", encryptImageRef).AssertOutContains("org.opencontainers.image.enc.keys.jwe")
	base.Cmd("push", encryptImageRef).AssertOK()
	// remove all local images (in the nerdctl-test namespace), to ensure that we do not have blobs of the original image.
	utils.RmiAll(base)
	base.Cmd("pull", encryptImageRef).AssertFail() // defaults to --unpack=true, and fails due to missing Prv key
	base.Cmd("pull", "--unpack=false", encryptImageRef).AssertOK()
	decryptImageRef := tID + ":decrypted"
	defer base.Cmd("rmi", decryptImageRef).Run()
	base.Cmd("image", "decrypt", "--key="+keyPair.Pub, encryptImageRef, decryptImageRef).AssertFail() // decryption needs Prv key, not Pub key
	base.Cmd("image", "decrypt", "--key="+keyPair.Prv, encryptImageRef, decryptImageRef).AssertOK()
}
