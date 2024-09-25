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
)

/*
This test below is meant to assert that https://github.com/containerd/nerdctl/issues/827 is NOT fixed.
Obviously, once we fix the issue, it should be replaced by something that assert it works.
Unfortunately, this is flaky.
It will regularly succeed or fail, making random PR fail the Kube check.
*/

func TestKubeCommitPush(t *testing.T) {
	t.Parallel()

	base := testutil.NewBaseForKubernetes(t)
	tID := testutil.Identifier(t)

	var containerID string
	// var registryIP string

	setup := func() {
		testutil.KubectlHelper(base, "run", "--image", testutil.CommonImage, tID, "--", "sleep", "Inf").
			AssertOK()

		testutil.KubectlHelper(base, "wait", "pod", tID, "--for=condition=ready", "--timeout=1m").
			AssertOK()

		testutil.KubectlHelper(base, "exec", tID, "--", "mkdir", "-p", "/tmp/whatever").
			AssertOK()

		cmd := testutil.KubectlHelper(base, "get", "pods", tID, "-o", "jsonpath={ .status.containerStatuses[0].containerID }")
		cmd.Run()
		containerID = strings.TrimPrefix(cmd.Out(), "containerd://")

		// This below is missing configuration to allow for plain http communication
		// This is left here for future work to successfully start a registry usable in the cluster
		/*
			// Start a registry
					testutil.KubectlHelper(base, "run", "--port", "5000", "--image", testutil.RegistryImageStable, "testregistry").
						AssertOK()

					testutil.KubectlHelper(base, "wait", "pod", "testregistry", "--for=condition=ready", "--timeout=1m").
						AssertOK()

					cmd = testutil.KubectlHelper(base, "get", "pods", tID, "-o", "jsonpath={ .status.hostIPs[0].ip }")
					cmd.Run()
					registryIP = cmd.Out()

					cmd = testutil.KubectlHelper(base, "apply", "-f", "-", fmt.Sprintf(`apiVersion: v1
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

	tearDown := func() {
		testutil.KubectlHelper(base, "delete", "pod", "--all").Run()
	}

	tearDown()
	t.Cleanup(tearDown)
	setup()

	t.Run("test commit / push on Kube (https://github.com/containerd/nerdctl/issues/827)", func(t *testing.T) {
		base.Cmd("commit", containerID, "testcommitsave").AssertOK()
		base.Cmd("save", "testcommitsave").AssertOK()
	})
}
