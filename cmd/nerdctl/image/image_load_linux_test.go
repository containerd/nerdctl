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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestLoadStdinFromPipe(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	img := testutil.Identifier(t)
	tmp := t.TempDir()
	output := filepath.Join(tmp, "output")

	setup := func() {
		base.Cmd("pull", testutil.CommonImage).AssertOK()
		base.Cmd("tag", testutil.CommonImage, img).AssertOK()
		base.Cmd("save", img, "-o", filepath.Join(tmp, "common.tar")).AssertOK()
		base.Cmd("rmi", "-f", img).AssertOK()
	}

	tearDown := func() {
		base.Cmd("rmi", "-f", img).AssertOK()
	}

	t.Cleanup(tearDown)
	tearDown()

	setup()

	loadCmd := strings.Join(base.Cmd("load").Command, " ")
	combined, err := exec.Command("sh", "-euxc", fmt.Sprintf("`cat %s/common.tar | %s > %s`", tmp, loadCmd, output)).CombinedOutput()
	assert.NilError(t, err, "failed with error %s and combined output is %s", err, string(combined))

	fb, err := os.ReadFile(output)
	assert.NilError(t, err)

	assert.Assert(t, strings.Contains(string(fb), fmt.Sprintf("Loaded image: %s:latest", img)))
	base.Cmd("images").AssertOutContains(img)
}

func TestLoadStdinEmpty(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("load").AssertFail()
}
