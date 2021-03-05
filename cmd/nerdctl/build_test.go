/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/AkihiroSuda/nerdctl/pkg/buildkitutil"
	"github.com/AkihiroSuda/nerdctl/pkg/defaults"
	"github.com/AkihiroSuda/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestBuild(t *testing.T) {
	base := testutil.NewBase(t)
	if base.Target == testutil.Nerdctl {
		buildkitHost := defaults.BuildKitHost()
		t.Logf("buildkitHost=%q", buildkitHost)
		if err := buildkitutil.PingBKDaemon(buildkitHost); err != nil {
			t.Skipf("test requires buildkitd: %+v", err)
		}
	}

	const imageName = "nerdctl-build-test"
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOut("nerdctl-build-test-string")
}

func createBuildContext(dockerfile string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "nerdctl-build-test")
	if err != nil {
		return "", err
	}
	if err = ioutil.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return "", err
	}
	return tmpDir, nil
}
