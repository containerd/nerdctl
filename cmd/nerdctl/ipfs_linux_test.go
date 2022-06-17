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
	tID := testutil.Identifier(t)
	encryptImageRef := tID + ":enc"
	layersNum := 1
	base.Cmd("image", "encrypt", "--recipient=jwe:"+keyPair.pub, ipfsCID, encryptImageRef).AssertOK()
	base.Cmd("image", "inspect", "--mode=native", "--format={{len .Manifest.Layers}}", encryptImageRef).AssertOutExactly(fmt.Sprintf("%d\n", layersNum))
	for i := 0; i < layersNum; i++ {
		base.Cmd("image", "inspect", "--mode=native", fmt.Sprintf("--format={{json (index .Manifest.Layers %d) }}", i), encryptImageRef).AssertOutContains("org.opencontainers.image.enc.keys.jwe")
	}
	ipfsCIDEnc := cidOf(t, base.Cmd("push", "ipfs://"+encryptImageRef).OutLines())
	rmiAll(base)

	decryptImageRef := tID + ":dec"
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
	tID := testutil.Identifier(t)
	newContainer, newImg := tID, tID+":v1"
	base.Cmd("run", "--name", newContainer, "-d", ipfsCID, "/bin/sh", "-c", "echo hello > /hello ; sleep 10000").AssertOK()
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
	requiresStargz(base)
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
	requiresStargz(base)
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage, "--estargz")

	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=stargz")
	base.Cmd("pull", ipfsCID).AssertOK()
	base.Cmd("run", "--rm", ipfsCID, "ls", "/.stargz-snapshotter").AssertOK()
	tID := testutil.Identifier(t)
	newContainer, newImg := tID, tID+":v1"
	base.Cmd("run", "--name", newContainer, "-d", ipfsCID, "/bin/sh", "-c", "echo hello > /hello ; sleep 10000").AssertOK()
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
}
