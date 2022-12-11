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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/testutil"

	"gotest.tools/v3/assert"
)

func TestSave(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	archiveTarPath := filepath.Join(t.TempDir(), "a.tar")
	base.Cmd("save", "-o", archiveTarPath, testutil.AlpineImage).AssertOK()
	rootfsPath := filepath.Join(t.TempDir(), "rootfs")
	err := utils.ExtractDockerArchive(archiveTarPath, rootfsPath)
	assert.NilError(t, err)
	etcOSReleasePath := filepath.Join(rootfsPath, "/etc/os-release")
	etcOSReleaseBytes, err := os.ReadFile(etcOSReleasePath)
	assert.NilError(t, err)
	etcOSRelease := string(etcOSReleaseBytes)
	t.Logf("read %q, extracted from %q", etcOSReleasePath, testutil.AlpineImage)
	t.Log(etcOSRelease)
	assert.Assert(t, strings.Contains(etcOSRelease, "Alpine"))
}
