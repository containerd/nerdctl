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
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"

	"gotest.tools/v3/assert"
)

func TestIPFS(t *testing.T) {
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

var iplineRegexp = regexp.MustCompile(`"([0-9\.]*)"`)

func TestIPFSAddress(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	ipfsaddr, done := runIPFSDaemonContainer(t, base)
	defer done()
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage, fmt.Sprintf("--ipfs-address=%s", ipfsaddr))
	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=overlayfs")
	base.Cmd("pull", "--ipfs-address", ipfsaddr, ipfsCID).AssertOK()
	base.Cmd("run", "--ipfs-address", ipfsaddr, "--rm", ipfsCID, "echo", "hello").AssertOK()
}

func runIPFSDaemonContainer(t *testing.T, base *testutil.Base) (ipfsAddress string, done func()) {
	name := "test-ipfs-address"
	var ipfsaddr string
	if detachedNetNS, _ := rootlessutil.DetachedNetNS(); detachedNetNS != "" {
		// detached-netns mode can't use .NetworkSettings.IPAddress, because the daemon and CNI has different network namespaces
		base.Cmd("run", "-d", "-p", "127.0.0.1:5999:5999", "--name", name, "--entrypoint=/bin/sh", testutil.KuboImage, "-c", "ipfs init && ipfs config Addresses.API /ip4/0.0.0.0/tcp/5999 && ipfs daemon --offline").AssertOK()
		ipfsaddr = "/ip4/127.0.0.1/tcp/5999"
	} else {
		base.Cmd("run", "-d", "--name", name, "--entrypoint=/bin/sh", testutil.KuboImage, "-c", "ipfs init && ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001 && ipfs daemon --offline").AssertOK()
		iplines := base.Cmd("inspect", name, "-f", "'{{json .NetworkSettings.IPAddress}}'").OutLines()
		t.Logf("IPAddress=%v", iplines)
		assert.Equal(t, len(iplines), 2)
		matches := iplineRegexp.FindStringSubmatch(iplines[0])
		t.Logf("ip address matches=%v", matches)
		assert.Equal(t, len(matches), 2)
		ipfsaddr = fmt.Sprintf("/ip4/%s/tcp/5001", matches[1])
	}
	return ipfsaddr, func() {
		base.Cmd("kill", "test-ipfs-address").AssertOK()
		base.Cmd("rm", "test-ipfs-address").AssertOK()
	}
}

func TestIPFSCommit(t *testing.T) {
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
	base.Cmd("kill", newContainer).AssertOK()
	base.Cmd("rm", newContainer).AssertOK()
	ipfsCID2 := cidOf(t, base.Cmd("push", "ipfs://"+newImg).OutLines())
	rmiAll(base)
	base.Cmd("pull", ipfsCID2).AssertOK()
	base.Cmd("run", "--rm", ipfsCID2, "/bin/sh", "-c", "cat /hello").AssertOK()
}

func TestIPFSWithLazyPulling(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	requiresStargz(base)
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage, "--estargz")

	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=stargz")
	base.Cmd("pull", ipfsCID).AssertOK()
	base.Cmd("run", "--rm", ipfsCID, "ls", "/.stargz-snapshotter").AssertOK()
}

func TestIPFSWithLazyPullingCommit(t *testing.T) {
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
	base.Cmd("kill", newContainer).AssertOK()
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
	return "ipfs://" + lines[len(lines)-2]
}
