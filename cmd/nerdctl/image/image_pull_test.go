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
	"strings"
	"testing"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestImagePullPlainHttpWithDefaultPort(t *testing.T) {
	nerdtest.Setup()

	var reg *registry.Server

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Registry,
			test.Not(nerdtest.Docker),
			nerdtest.Build,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			reg = nerdtest.RegistryWithNoAuth(data, helpers, 80, false)
			reg.Setup(data, helpers)

			testImageRef := fmt.Sprintf("%s/%s:%s",
				reg.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

			buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
			helpers.Ensure("build", "-t", testImageRef, buildCtx)
			helpers.Ensure("--insecure-registry", "push", testImageRef)
			helpers.Ensure("rmi", "-f", testImageRef)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			testImageRef := fmt.Sprintf("%s/%s:%s",
				reg.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
			return helpers.Command("--insecure-registry", "pull", testImageRef)
		},
		Expected: test.Expects(0, nil, nil),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if reg != nil {
				reg.Cleanup(data, helpers)
				testImageRef := fmt.Sprintf("%s/%s:%s",
					reg.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
				helpers.Anyhow("rmi", "-f", testImageRef)
			}
		},
	}

	testCase.Run(t)
}
