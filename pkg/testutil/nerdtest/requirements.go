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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/platform"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

var BuildkitHost test.ConfigKey = "bkHost"

// These are used for ambient requirements
var ipv6 test.ConfigKey = "IPv6Test"
var kubernetes test.ConfigKey = "KubeTest"
var flaky test.ConfigKey = "FlakyTest"
var only test.ConfigValue = "Only"

// These are used for down the road configuration and custom behavior inside command
var modePrivate test.ConfigKey = "PrivateMode"
var stargz test.ConfigKey = "Stargz"
var ipfs test.ConfigKey = "IPFS"
var enabled test.ConfigValue = "Enabled"

// OnlyIPv6 marks a test as suitable to be run exclusively inside an ipv6 environment
// This is an ambient requirement
var OnlyIPv6 = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		helpers.Write(ipv6, only)
		ret = environmentHasIPv6()
		if !ret {
			mess = "runner skips IPv6 compatible tests in the non-IPv6 environment"
		}
		return ret, mess
	},
}

// OnlyKubernetes marks a test as meant to be tested on Kubernetes
// This is an ambient requirement
var OnlyKubernetes = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		helpers.Write(kubernetes, only)
		ret = environmentHasKubernetes()
		if !ret {
			mess = "runner skips Kubernetes compatible tests in the non-Kubernetes environment"
		}
		return ret, mess
	},
}

// IsFlaky marks a test as randomly failing.
// This is an ambient requirement
var IsFlaky = func(issueLink string) *test.Requirement {
	return &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
			// We do not even want to get to the setup phase here
			helpers.Write(flaky, only)
			ret = environmentIsForFlaky()
			if !ret {
				mess = "runner skips flaky compatible tests in the non-flaky environment"
			}
			return ret, mess
		},
	}
}

// Docker marks a test as suitable solely for Docker and not Nerdctl
// Generally used as test.Not(nerdtest.Docker), which of course it the opposite
var Docker = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		ret = getTarget() == targetDocker
		if ret {
			mess = "current target is docker"
		} else {
			mess = "current target is not docker"
		}
		return ret, mess
	},
}

// NerdctlNeedsFixing marks a test as unsuitable to be run for Nerdctl, because of a specific known issue which
// url must be passed as an argument
var NerdctlNeedsFixing = func(issueLink string) *test.Requirement {
	return &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
			ret = getTarget() == targetDocker
			if ret {
				mess = "current target is docker"
			} else {
				mess = "current target is nerdctl, but we will skip as nerdctl currently has issue: " + issueLink
			}
			return ret, mess
		},
	}
}

// BrokenTest marks a test as currently broken, with explanation provided in message, along with
// additional requirements / restrictions describing what it can run on.
var BrokenTest = func(message string, req *test.Requirement) *test.Requirement {
	return &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers) (bool, string) {
			ret, mess := req.Check(data, helpers)
			return ret, message + "\n" + mess
		},
		Setup:   req.Setup,
		Cleanup: req.Cleanup,
	}
}

// RootLess marks a test as suitable only for the rootless environment
var RootLess = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		// Make sure we DO not return "IsRootless true" for docker
		ret = getTarget() == targetNerdctl && rootlessutil.IsRootless()
		if ret {
			mess = "environment is root-less"
		} else {
			mess = "environment is root-ful"
		}
		return ret, mess
	},
}

// RootFul marks a test as suitable only for rootful env
var RootFul = test.Not(RootLess)

// CGroup requires that cgroup is enabled
var CGroup = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		ret = true
		mess = "cgroup is enabled"
		stdout := helpers.Capture("info", "--format", "{{ json . }}")
		var dinf dockercompat.Info
		err := json.Unmarshal([]byte(stdout), &dinf)
		assert.NilError(helpers.T(), err, "failed to parse docker info")
		switch dinf.CgroupDriver {
		case "none", "":
			ret = false
			mess = "cgroup is none"
		}
		return ret, mess
	},
}

// Soci requires that the soci snapshotter is enabled
var Soci = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		ret = false
		mess = "soci is not enabled"
		stdout := helpers.Capture("info", "--format", "{{ json . }}")
		var dinf dockercompat.Info
		err := json.Unmarshal([]byte(stdout), &dinf)
		assert.NilError(helpers.T(), err, "failed to parse docker info")
		for _, p := range dinf.Plugins.Storage {
			if p == "soci" {
				ret = true
				mess = "soci is enabled"
			}
		}
		return ret, mess
	},
}

var Stargz = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		ret = false
		mess = "stargz is not enabled"
		stdout := helpers.Capture("info", "--format", "{{ json . }}")
		var dinf dockercompat.Info
		err := json.Unmarshal([]byte(stdout), &dinf)
		assert.NilError(helpers.T(), err, "failed to parse docker info")
		for _, p := range dinf.Plugins.Storage {
			if p == "stargz" {
				ret = true
				mess = "stargz is enabled"
			}
		}
		// Need this to happen now for Cleanups to work
		// FIXME: we should be able to access the env (at least through helpers.Command().) instead of this gym
		helpers.Write(stargz, enabled)
		return ret, mess
	},
}

// Registry marks a test as requiring a registry to be deployed
var Registry = test.Require(
	// Registry requires Linux currently
	test.Linux,
	(func() *test.Requirement {
		// Provisional: see note in cleanup
		// var reg *registry.Server

		return &test.Requirement{
			Check: func(data test.Data, helpers test.Helpers) (bool, string) {
				return true, ""
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				// Ensure we have registry images now, so that we can run --pull=never
				// This is useful for two reasons:
				// - if ghcr.io is out, we want to fail early
				// - when we start a large number of registries in subtests, no need to round-trip to ghcr everytime
				// This of course assumes that the subtests are NOT going to prune / rmi images
				registryImage := platform.RegistryImageStable
				up := os.Getenv("DISTRIBUTION_VERSION")
				if up != "" {
					if up[0:1] != "v" {
						up = "v" + up
					}
					registryImage = platform.RegistryImageNext + up
				}
				helpers.Ensure("pull", "--quiet", registryImage)
				helpers.Ensure("pull", "--quiet", platform.DockerAuthImage)
				helpers.Ensure("pull", "--quiet", platform.KuboImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// FIXME: figure out what to do with reg setup/cleanup routines
				// Provisionally, reg is available here in the closure
			},
		}
	})(),
)

// Build marks a test as suitable only if buildkitd is enabled (only tested for nerdctl obviously)
var Build = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (bool, string) {
		// FIXME: shouldn't we run buildkitd in a container? At least for testing, that would be so much easier than
		// against the host install
		ret := true
		mess := "buildkitd is enabled"

		if getTarget() == targetNerdctl {
			bkHostAddr, err := buildkitutil.GetBuildkitHost(defaultNamespace)
			if err != nil {
				ret = false
				mess = fmt.Sprintf("buildkitd is not enabled: %+v", err)
				return ret, mess
			}
			// We also require the buildctl binary in the path
			_, err = exec.LookPath("buildctl")
			if err != nil {
				ret = false
				mess = fmt.Sprintf("buildctl is not in the path: %+v", err)
				return ret, mess
			}
			helpers.Write(BuildkitHost, test.ConfigValue(bkHostAddr))
		}
		return ret, mess
	},
	Cleanup: func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("builder", "prune", "--all", "--force")
	},
}

var IPFS = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		// FIXME: we should be able to access the env (at least through helpers.Command().) instead of this gym
		helpers.Write(ipfs, enabled)
		// FIXME: this is incomplete. We obviously need a daemon running, properly configured
		return test.Binary("ipfs").Check(data, helpers)
	},
}

// Private makes a test run inside a dedicated namespace, with a private config.toml, hosts directory, and DOCKER_CONFIG path
// If the target is docker, parallelism is forcefully disabled
var Private = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (ret bool, mess string) {
		// We need this to happen NOW and not in setup, as otherwise cleanup with operate on the default namespace
		namespace := data.Identifier("private")
		helpers.Write(Namespace, test.ConfigValue(namespace))
		data.Set("_deletenamespace", namespace)
		// FIXME: is this necessary? Should NoParallel be subsumed into config?
		helpers.Write(modePrivate, enabled)
		return true, "private mode creates a dedicated namespace for nerdctl, and disable parallelism for docker"
	},

	Cleanup: func(data test.Data, helpers test.Helpers) {
		if getTarget() == targetNerdctl {
			// FIXME: there are conditions where we still have some stuff in there and this fails...
			containerList := strings.TrimSpace(helpers.Capture("ps", "-aq"))
			if containerList != "" {
				helpers.Ensure(append([]string{"rm", "-f"}, strings.Split(containerList, "\n")...)...)
			}
			helpers.Ensure("system", "prune", "-f", "--all", "--volumes")
			helpers.Anyhow("namespace", "remove", data.Get("_deletenamespace"))
		}
	},
}
