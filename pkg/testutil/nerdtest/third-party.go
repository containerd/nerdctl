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
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/ca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func BuildCtlCommand(data test.Data, helpers test.Helpers, args ...string) test.Command {
	// As tests with build Require Build, we already know we have buildctl in the path
	buildctl, _ := exec.LookPath("buildctl")
	bh, _ := data.Surface(BuildkitHost)
	cmd := helpers.CustomCommand(buildctl, "--addr="+string(bh))
	cmd.WithArgs(args...)
	return cmd
}

func RegistryWithTokenAuth(data test.Data, helpers test.Helpers, t *testing.T, user, pass string, port int, tls bool) (*registry.Server, *registry.TokenAuthServer) {
	rca := ca.New(data, t)
	as := registry.NewCesantaAuthServer(data, helpers, t, rca, 0, user, pass, tls)
	re := registry.NewDockerRegistry(data, helpers, t, rca, port, as.Auth)
	return re, as
}

func RegistryWithNoAuth(data test.Data, helpers test.Helpers, t *testing.T, port int, tls bool) *registry.Server {
	var rca *ca.CA
	if tls {
		rca = ca.New(data, t)
	}
	return registry.NewDockerRegistry(data, helpers, t, rca, port, &registry.NoAuth{})
}

func RegistryWithBasicAuth(data test.Data, helpers test.Helpers, t *testing.T, user, pass string, port int, tls bool) *registry.Server {
	auth := &registry.BasicAuth{
		Username: user,
		Password: pass,
	}
	var rca *ca.CA
	if tls {
		rca = ca.New(data, t)
	}
	return registry.NewDockerRegistry(data, helpers, t, rca, port, auth)
}

/*
	if r, err := os.Open(tomlPath); err == nil {
		log.L.Debugf("Loading config from %q", tomlPath)
		defer r.Close()
		dec := toml.NewDecoder(r).DisallowUnknownFields() // set Strict to detect typo
		if err := dec.Decode(cfg); err != nil {
			return nil, fmt.Errorf("failed to load nerdctl config (not daemon config) from %q (Hint: don't mix up daemon's `config.toml` with `nerdctl.toml`): %w", tomlPath, err)
		}
		log.L.Debugf("Loaded config %+v", cfg)

*/
