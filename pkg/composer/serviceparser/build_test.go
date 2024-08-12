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

package serviceparser

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func lastOf(ss []string) string {
	return ss[len(ss)-1]
}

func TestParseBuild(t *testing.T) {
	t.Parallel()
	const dockerComposeYAML = `
services:
  foo:
    build: ./fooctx
    pull_policy: always
  bar:
    image: barimg
    pull_policy: build
    build:
      context: ./barctx
      target: bartgt
      labels:
        bar: baz
      secrets:
        - source: src_secret
          target: tgt_secret
        - simple_secret
        - absolute_secret
secrets:
  src_secret:
    file: test_secret1
  simple_secret:
    file: test_secret2
  absolute_secret:
    file: /tmp/absolute_secret
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := testutil.LoadProject(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	fooSvc, err := project.GetService("foo")
	assert.NilError(t, err)

	foo, err := Parse(project, fooSvc)
	assert.NilError(t, err)

	t.Logf("foo: %+v", foo)
	assert.Equal(t, DefaultImageName(project.Name, "foo"), foo.Image)
	assert.Equal(t, false, foo.Build.Force)
	assert.Equal(t, project.RelativePath("fooctx"), lastOf(foo.Build.BuildArgs))

	barSvc, err := project.GetService("bar")
	assert.NilError(t, err)

	bar, err := Parse(project, barSvc)
	assert.NilError(t, err)

	t.Logf("bar: %+v", bar)
	assert.Equal(t, "barimg", bar.Image)
	assert.Equal(t, true, bar.Build.Force)
	assert.Equal(t, project.RelativePath("barctx"), lastOf(bar.Build.BuildArgs))
	assert.Assert(t, in(bar.Build.BuildArgs, "--target=bartgt"))
	assert.Assert(t, in(bar.Build.BuildArgs, "--label=bar=baz"))
	secretPath := project.WorkingDir
	assert.Assert(t, in(bar.Build.BuildArgs, "--secret=id=tgt_secret,src="+secretPath+"/test_secret1"))
	assert.Assert(t, in(bar.Build.BuildArgs, "--secret=id=simple_secret,src="+secretPath+"/test_secret2"))
	assert.Assert(t, in(bar.Build.BuildArgs, "--secret=id=absolute_secret,src=/tmp/absolute_secret"))
}
