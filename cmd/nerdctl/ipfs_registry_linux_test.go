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
	"os"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestIPFSRegistry(t *testing.T) {
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=overlayfs")
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage)
	ipfsRegistryAddr := "localhost:5555"
	ipfsRegistryRef := ipfsRegistryReference(ipfsRegistryAddr, ipfsCID)

	done := ipfsRegistryUp(t, base, "--listen-registry", ipfsRegistryAddr)
	defer done()
	base.Cmd("pull", ipfsRegistryRef).AssertOK()
	base.Cmd("run", "--rm", ipfsRegistryRef, "echo", "hello").AssertOK()
}

func TestIPFSRegistryWithLazyPulling(t *testing.T) {
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	requiresStargz(base)
	base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER=stargz")
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage, "--estargz")
	ipfsRegistryAddr := "localhost:5555"
	ipfsRegistryRef := ipfsRegistryReference(ipfsRegistryAddr, ipfsCID)

	done := ipfsRegistryUp(t, base, "--listen-registry", ipfsRegistryAddr)
	defer done()
	base.Cmd("pull", ipfsRegistryRef).AssertOK()
	base.Cmd("run", "--rm", ipfsRegistryRef, "ls", "/.stargz-snapshotter").AssertOK()
}

func ipfsRegistryReference(addr string, c string) string {
	return addr + "/ipfs/" + strings.TrimPrefix(c, "ipfs://")
}
