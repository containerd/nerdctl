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

package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestSave(t *testing.T) {
	// See detailed comment in TestRunCustomRootfs for why we need a separate namespace.
	base := testutil.NewBaseWithNamespace(t, testutil.Identifier(t))
	t.Cleanup(func() {
		base.Cmd("namespace", "remove", testutil.Identifier(t)).Run()
	})
	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	archiveTarPath := filepath.Join(t.TempDir(), "a.tar")
	base.Cmd("save", "-o", archiveTarPath, testutil.AlpineImage).AssertOK()
	rootfsPath := filepath.Join(t.TempDir(), "rootfs")
	err := helpers.ExtractDockerArchive(archiveTarPath, rootfsPath)
	assert.NilError(t, err)
	etcOSReleasePath := filepath.Join(rootfsPath, "/etc/os-release")
	etcOSReleaseBytes, err := os.ReadFile(etcOSReleasePath)
	assert.NilError(t, err)
	etcOSRelease := string(etcOSReleaseBytes)
	t.Logf("read %q, extracted from %q", etcOSReleasePath, testutil.AlpineImage)
	t.Log(etcOSRelease)
	assert.Assert(t, strings.Contains(etcOSRelease, "Alpine"))
}
