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

	"github.com/containerd/nerdctl/pkg/composer/projectloader"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
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
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := projectloader.Load(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	fooSvc, err := project.GetService("foo")
	assert.NilError(t, err)

	foo, err := Parse(project, fooSvc)
	assert.NilError(t, err)

	t.Logf("foo: %+v", foo)
	assert.Equal(t, project.Name+"_foo", foo.Image)
	assert.Equal(t, false, foo.Build.Force)
	assert.Equal(t, project.RelativePath("fooctx"), lastOf(foo.Build.BuildArgs))

	barSvc, err := project.GetService("bar")
	assert.NilError(t, err)

	bar, err := Parse(project, barSvc)
	assert.NilError(t, err)

	t.Logf("bar: %+v", foo)
	assert.Equal(t, "barimg", bar.Image)
	assert.Equal(t, true, bar.Build.Force)
	assert.Equal(t, project.RelativePath("barctx"), lastOf(bar.Build.BuildArgs))
	assert.Assert(t, in(bar.Build.BuildArgs, "--target=bartgt"))
}
