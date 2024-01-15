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
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestBuilderDebug(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-builder-debug-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("builder", "debug", buildCtx).CmdOption(testutil.WithStdin(bytes.NewReader([]byte("c\n")))).AssertOK()
}
