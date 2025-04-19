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

package container

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

func TestRunVerifyCosign(t *testing.T) {
	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	var reg *registry.Server

	testCase.Require = require.All(
		require.Binary("cosign"),
		require.Not(nerdtest.Docker),
		nerdtest.Build,
		nerdtest.Registry,
	)

	testCase.Env["COSIGN_PASSWORD"] = "1"

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerfile, "Dockerfile")
		pri, pub := nerdtest.GenerateCosignKeyPair(data, helpers, "1")
		reg = nerdtest.RegistryWithNoAuth(data, helpers, 0, false)
		reg.Setup(data, helpers)

		testImageRef := fmt.Sprintf("127.0.0.1:%d/%s", reg.Port, data.Identifier("push-cosign-image"))
		helpers.Ensure("build", "-t", testImageRef, data.Temp().Path())
		helpers.Ensure("push", testImageRef, "--sign=cosign", "--cosign-key="+pri)

		data.Labels().Set("public_key", pub)
		data.Labels().Set("image_ref", testImageRef)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if reg != nil {
			reg.Cleanup(data, helpers)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		helpers.Fail(
			"run", "--rm", "--verify=cosign",
			"--cosign-key=dummy",
			data.Labels().Get("image_ref"))

		return helpers.Command(
			"run", "--rm", "--verify=cosign",
			"--cosign-key="+data.Labels().Get("public_key"),
			data.Labels().Get("image_ref"))
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}
