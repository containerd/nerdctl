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

package nerdtest

import (
	"os/exec"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/ca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func BuildCtlCommand(helpers test.Helpers, args ...string) test.TestableCommand {
	assert.Assert(helpers.T(), string(helpers.Read(BuildkitHost)) != "", "You first need to Require Build to use buildctl")
	buildctl, _ := exec.LookPath("buildctl")
	cmd := helpers.Custom(buildctl)
	cmd.WithArgs("--addr=" + string(helpers.Read(BuildkitHost)))
	cmd.WithArgs(args...)
	return cmd
}

func KubeCtlCommand(helpers test.Helpers, args ...string) test.TestableCommand {
	kubectl, _ := exec.LookPath("kubectl")
	cmd := helpers.Custom(kubectl)
	cmd.WithArgs("--namespace=nerdctl-test-k8s")
	cmd.WithArgs(args...)
	return cmd
}

func RegistryWithTokenAuth(data test.Data, helpers test.Helpers, user, pass string, port int, tls bool) (*registry.Server, *registry.TokenAuthServer) {
	rca := ca.New(data, helpers.T())
	as := registry.NewCesantaAuthServer(data, helpers, rca, 0, user, pass, tls)
	re := registry.NewDockerRegistry(data, helpers, rca, port, as.Auth)
	return re, as
}

func RegistryWithNoAuth(data test.Data, helpers test.Helpers, port int, tls bool) *registry.Server {
	var rca *ca.CA
	if tls {
		rca = ca.New(data, helpers.T())
	}
	return registry.NewDockerRegistry(data, helpers, rca, port, &registry.NoAuth{})
}

func RegistryWithBasicAuth(data test.Data, helpers test.Helpers, user, pass string, port int, tls bool) *registry.Server {
	auth := &registry.BasicAuth{
		Username: user,
		Password: pass,
	}
	var rca *ca.CA
	if tls {
		rca = ca.New(data, helpers.T())
	}
	return registry.NewDockerRegistry(data, helpers, rca, port, auth)
}
