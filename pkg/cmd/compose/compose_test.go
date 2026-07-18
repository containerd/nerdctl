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

package compose

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

// TestImageVerifyOptionsFromComposeNonStringExtension makes sure a non-string
// x-nerdctl-verify value in a compose file does not crash the CLI. compose-go
// accepts any value under an x-* key, so a user typo like `x-nerdctl-verify: 123`
// used to reach an unguarded type assertion and panic with
// "interface conversion: interface {} is int, not string".
func TestImageVerifyOptionsFromComposeNonStringExtension(t *testing.T) {
	const dockerComposeYAML = `
services:
  app:
    image: alpine:latest
    x-nerdctl-verify: 123
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := testutil.LoadProject(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	svcConfig, err := project.GetService("app")
	assert.NilError(t, err)

	ps, err := serviceparser.Parse(project, svcConfig)
	assert.NilError(t, err)

	opt := imageVerifyOptionsFromCompose(ps)
	// A non-string verify value is ignored and falls back to the default.
	assert.Equal(t, "none", opt.Provider)
}
