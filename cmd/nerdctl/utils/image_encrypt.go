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

package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

type JweKeyPair struct {
	Prv     string
	Pub     string
	Cleanup func()
}

func NewJWEKeyPair(t testing.TB) *JweKeyPair {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip(err)
	}
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
	imageIDs := base.Cmd("images", "--no-trunc", "-a", "-q").OutLines()
	base.Cmd(append([]string{"rmi", "-f"}, imageIDs...)...).AssertOK()
	if _, err := buildkitutil.GetBuildkitHost(testutil.Namespace); err == nil {
		base.Cmd("builder", "prune").AssertOK()
	}
}
