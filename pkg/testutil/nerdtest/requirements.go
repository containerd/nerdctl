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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

var ipv6 test.ConfigKey = "IPv6Test"
var kubernetes test.ConfigKey = "KubeTest"
var flaky test.ConfigKey = "FlakyTest"
var only test.ConfigValue = "Only"
var mode test.ConfigKey = "Mode"
var modePrivate test.ConfigValue = "Private"

func environmentHasIPv6() bool {
	return testutil.GetEnableIPv6()
}

func environmentHasKubernetes() bool {
	return testutil.GetEnableKubernetes()
}

func environmentIsForFlaky() bool {
	return testutil.GetEnableIPv6()
}

// OnlyIPv6 marks a test as suitable to be run exclusively inside an ipv6 environment
var OnlyIPv6 = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		ret = environmentHasIPv6()
		if !ret {
			mess = "runner skips IPv6 compatible tests in the non-IPv6 environment"
		}
		data.WithConfig(ipv6, only)
		return ret, mess
	},
}

var OnlyKubernetes = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		ret = environmentHasKubernetes()
		if !ret {
			mess = "runner skips Kubernetes compatible tests in the non-Kubernetes environment"
		}
		data.WithConfig(kubernetes, only)
		return ret, mess
	},
}

// Docker marks a test as suitable solely for Docker and not Nerdctl
// Generally used as test.Not(nerdtest.Docker), which of course it the opposite
var Docker = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		ret = GetTarget() == TargetDocker
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
		Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
			ret = GetTarget() == TargetDocker
			if ret {
				mess = "current target is docker"
			} else {
				mess = "current target is nerdctl, but we will skip as it is currently broken: " + issueLink
			}
			return ret, mess
		},
	}
}

var IsFlaky = func(issueLink string) *test.Requirement {
	return &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
			ret = environmentIsForFlaky()
			if !ret {
				mess = "runner skips flaky compatible tests in the non-flaky environment"
			}
			data.WithConfig(flaky, only)
			return ret, mess
		},
	}
}

// BrokenTest marks a test as currently broken, with explanation provided in message, along with
// additional requirements / restrictions describing what it can run on.
var BrokenTest = func(message string, req *test.Requirement) *test.Requirement {
	return &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers, t *testing.T) (bool, string) {
			ret, mess := req.Check(data, helpers, t)
			return ret, message + "\n" + mess
		},
		Setup:   req.Setup,
		Cleanup: req.Cleanup,
	}
}

// RootLess marks a test as suitable only for the rootless environment
var RootLess = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		// Make sure we DO not return "IsRootless true" for docker
		ret = GetTarget() == targetNerdctl && rootlessutil.IsRootless()
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
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		ret = true
		mess = "cgroup is enabled"
		stdout := helpers.Capture("info", "--format", "{{ json . }}")
		var dinf dockercompat.Info
		err := json.Unmarshal([]byte(stdout), &dinf)
		assert.NilError(t, err, "failed to parse docker info")
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
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		ret = false
		mess = "soci is not enabled"
		stdout := helpers.Capture("info", "--format", "{{ json . }}")
		var dinf dockercompat.Info
		err := json.Unmarshal([]byte(stdout), &dinf)
		assert.NilError(t, err, "failed to parse docker info")
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
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		ret = false
		mess = "soci is not enabled"
		stdout := helpers.Capture("info", "--format", "{{ json . }}")
		var dinf dockercompat.Info
		err := json.Unmarshal([]byte(stdout), &dinf)
		assert.NilError(t, err, "failed to parse docker info")
		for _, p := range dinf.Plugins.Storage {
			if p == "stargz" {
				ret = true
				mess = "stargz is enabled"
			}
		}
		return ret, mess
	},
}

// Registry marks a test as requiring a registry to be deployed
var Registry = test.Require(
	// Registry requires Linux currently
	test.Linux,
	&test.Requirement{
		Check: func(data test.Data, helpers test.Helpers, t *testing.T) (bool, string) {
			return true, ""
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			// Ensure we have registry images now, so that we can run --pull=never
			// This is useful for two reasons:
			// - if ghcr.io is out, we want to fail early
			// - when we start a large number of registries in subtests, no need to round-trip to ghcr everytime
			// This of course assumes that the subtests are NOT going to prune / rmi images
			registryImage := testutil.RegistryImageStable
			up := os.Getenv("DISTRIBUTION_VERSION")
			if up != "" {
				if up[0:1] != "v" {
					up = "v" + up
				}
				registryImage = testutil.RegistryImageNext + up
			}
			helpers.Ensure("pull", "--quiet", registryImage)
			helpers.Ensure("pull", "--quiet", testutil.DockerAuthImage)
			helpers.Ensure("pull", "--quiet", testutil.KuboImage)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			// XXX FIXME: figure out what to do with reg setup/cleanup routines
		},
	},
)

var BuildkitHost = test.SystemKey("bkHost")

// Build marks a test as suitable only if buildkitd is enabled (only tested for nerdctl obviously)
var Build = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (bool, string) {
		// FIXME: shouldn't we run buildkitd in a container? At least for testing, that would be so much easier than
		// against the host install
		ret := true
		mess := "buildkitd is enabled"

		if GetTarget() == targetNerdctl {
			// NOTE: we might not have the final namespace... as Private may be processed later
			ns, _ := data.Surface(test.SystemKey(Namespace))
			if ns == "" {
				ns = defaultNamespace
			}
			_, err := buildkitutil.GetBuildkitHost(string(ns))
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
		}
		return ret, mess
	},
	Setup: func(data test.Data, helpers test.Helpers) {
		ns, _ := data.Surface(test.SystemKey(Namespace))
		if ns == "" {
			ns = defaultNamespace
		}
		bkHostAddr, _ := buildkitutil.GetBuildkitHost(string(ns))
		data.Sink(BuildkitHost, test.SystemValue(bkHostAddr))
	},
	Cleanup: func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("builder", "prune", "--all", "--force")
	},
}

// Private makes a test run inside a dedicated namespace, with a private config.toml, hosts directory, and DOCKER_CONFIG path
// If the target is docker, parallelism is forcefully disabled
var Private = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers, t *testing.T) (ret bool, mess string) {
		// FIXME: is this necessary?
		data.WithConfig(mode, modePrivate)
		// That should be enough
		namespace := data.Identifier("private")
		data.WithConfig(Namespace, test.ConfigValue(namespace))
		// So... this happens too late
		return true, "private mode creates a dedicated namespace for nerdctl, and disable parallelism for docker"
	},
	Setup: func(data test.Data, helpers test.Helpers) {
		// SHOULD NoParallel be subsumed into regular config?
	},
	Cleanup: func(data test.Data, helpers test.Helpers) {
		containerList := helpers.Capture("ps", "-aq")
		helpers.Ensure("rm", "-f", containerList)
		helpers.Ensure("system", "prune", "-f", "--all", "--volumes")
		// FIXME: there are conditions where we still have some stuff in there
		helpers.Anyhow("namespace", "remove", string(data.ReadConfig(Namespace)))
	},
}
