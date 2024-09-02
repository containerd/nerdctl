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

package helpers

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func FindIPv6(output string) net.IP {
	var ipv6 string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "inet6") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				ipv6 = strings.Split(fields[1], "/")[0]
				break
			}
		}
	}
	return net.ParseIP(ipv6)
}

func RequiresStargz(base *testutil.Base) {
	info := base.Info()
	for _, p := range info.Plugins.Storage {
		if p == "stargz" {
			return
		}
	}
	base.T.Skip("test requires stargz")
}

type JweKeyPair struct {
	Prv     string
	Pub     string
	Cleanup func()
}

func NewJWEKeyPair(t testing.TB) *JweKeyPair {
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
	return &JweKeyPair{
		Prv: prv,
		Pub: pub,
		Cleanup: func() {
			_ = os.RemoveAll(td)
		},
	}
}

func RmiAll(base *testutil.Base) {
	base.T.Logf("Pruning images")
	imageIDs := base.Cmd("images", "--no-trunc", "-a", "-q").OutLines()
	// remove empty output line at the end
	imageIDs = imageIDs[:len(imageIDs)-1]
	// use `Run` on purpose (same below) because `rmi all` may fail on individual
	// image id that has an expected running container (e.g. a registry)
	base.Cmd(append([]string{"rmi", "-f"}, imageIDs...)...).Run()

	base.T.Logf("Pruning build caches")
	if _, err := buildkitutil.GetBuildkitHost(testutil.Namespace); err == nil {
		base.Cmd("builder", "prune", "--force").AssertOK()
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

func CreateBuildContext(t *testing.T, dockerfile string) string {
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644)
	assert.NilError(t, err)
	return tmpDir
}

func RequiresSoci(base *testutil.Base) {
	info := base.Info()
	for _, p := range info.Plugins.Storage {
		if p == "soci" {
			return
		}
	}
	base.T.Skip("test requires soci")
}

type CosignKeyPair struct {
	PublicKey  string
	PrivateKey string
	Cleanup    func()
}

func NewCosignKeyPair(t testing.TB, path string, password string) *CosignKeyPair {
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

	return &CosignKeyPair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
		Cleanup: func() {
			_ = os.RemoveAll(td)
		},
	}
}
