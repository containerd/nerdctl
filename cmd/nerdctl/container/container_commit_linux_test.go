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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestKubeCommitSave(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.OnlyKubernetes

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		containerID := ""
		// NOTE: kubectl namespaces are not the same as containerd namespaces.
		// We still want kube test objects segregated in their own Kube API namespace.
		nerdtest.KubeCtlCommand(helpers, "create", "namespace", "nerdctl-test-k8s").Run(&test.Expected{})
		nerdtest.KubeCtlCommand(helpers, "run", "--image", testutil.CommonImage, data.Identifier(), "--", "sleep", "Inf").Run(&test.Expected{})
		nerdtest.KubeCtlCommand(helpers, "wait", "pod", data.Identifier(), "--for=condition=ready", "--timeout=1m").Run(&test.Expected{})
		nerdtest.KubeCtlCommand(helpers, "exec", data.Identifier(), "--", "mkdir", "-p", "/tmp/whatever").Run(&test.Expected{})
		nerdtest.KubeCtlCommand(helpers, "get", "pods", data.Identifier(), "-o", "jsonpath={ .status.containerStatuses[0].containerID }").Run(&test.Expected{
			Output: func(stdout string, info string, t *testing.T) {
				containerID = strings.TrimPrefix(stdout, "containerd://")
			},
		})
		data.Set("containerID", containerID)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		nerdtest.KubeCtlCommand(helpers, "delete", "pod", "--all").Run(nil)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		helpers.Ensure("commit", data.Get("containerID"), "testcommitsave")
		return helpers.Command("save", "testcommitsave")
	}

	testCase.Expected = test.Expects(0, nil, nil)

	testCase.Run(t)

	// This below is missing configuration to allow for plain http communication
	// This is left here for future work to successfully start a registry usable in the cluster
	/*
		// Start a registry
				nerdtest.KubeCtlCommand(helpers, "run", "--port", "5000", "--image", testutil.RegistryImageStable, "testregistry").
					Run(&test.Expected{})

				nerdtest.KubeCtlCommand(helpers, "wait", "pod", "testregistry", "--for=condition=ready", "--timeout=1m").
					AssertOK()

				cmd = nerdtest.KubeCtlCommand(helpers, "get", "pods", tID, "-o", "jsonpath={ .status.hostIPs[0].ip }")
				cmd.Run(&test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						registryIP = stdout
					},
				})

				cmd = nerdtest.KubeCtlCommand(helpers, "apply", "-f", "-", fmt.Sprintf(`apiVersion: v1
			kind: ConfigMap
			metadata:
				name: local-registry
				namespace: nerdctl-test
			data:
				localRegistryHosting.v1: |
				host: "%s:5000"
				help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
		`, registryIP))
	*/
}
