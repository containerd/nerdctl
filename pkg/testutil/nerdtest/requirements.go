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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

var ipv6 test.ConfigKey = "IPv6Test"
var only test.ConfigValue = "Only"
var mode test.ConfigKey = "Mode"
var modePrivate test.ConfigValue = "Private"

var OnlyIPv6 = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	ret = testutil.GetEnableIPv6()
	if !ret {
		mess = "runner skips IPv6 compatible tests in the non-IPv6 environment"
	}
	data.WithConfig(ipv6, only)
	return ret, mess
})

var Private = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	data.WithConfig(mode, modePrivate)
	return true, "private mode"
})

var Soci = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	ret = false
	mess = "soci is not enabled"
	(&test.GenericCommand{}).
		WithT(t).
		WithBinary(testutil.GetTarget()).
		WithArgs("info", "--format", "{{ json . }}").
		Run(&test.Expected{
			Output: func(stdout string, info string, t *testing.T) {
				var dinf dockercompat.Info
				err := json.Unmarshal([]byte(stdout), &dinf)
				assert.NilError(t, err, "failed to parse docker info")
				for _, p := range dinf.Plugins.Storage {
					if p == "soci" {
						ret = true
						mess = "soci is enabled"
					}
				}
			},
		})

	return ret, mess
})

var Docker = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	ret = testutil.GetTarget() == testutil.Docker
	if ret {
		mess = "current target is docker"
	} else {
		mess = "current target is not docker"
	}
	return ret, mess
})

var NerdctlNeedsFixing = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	ret = testutil.GetTarget() == testutil.Docker
	if ret {
		mess = "current target is docker"
	} else {
		mess = "current target is nerdctl, but it is currently broken and not working for this"
	}
	return ret, mess
})

var Rootless = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	// Make sure we DO not return "IsRootless true" for docker
	ret = testutil.GetTarget() != testutil.Docker && rootlessutil.IsRootless()
	if ret {
		mess = "environment is rootless"
	} else {
		mess = "environment is rootful"
	}
	return ret, mess
})

var Build = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	// FIXME: shouldn't we run buildkitd in a container? At least for testing, that would be so much easier than
	// against the host install
	ret = true
	mess = "buildkitd is enabled"
	if testutil.GetTarget() == testutil.Nerdctl {
		_, err := buildkitutil.GetBuildkitHost(testutil.Namespace)
		if err != nil {
			ret = false
			mess = fmt.Sprintf("buildkitd is not enabled: %+v", err)
		}
	}
	return ret, mess
})

var CGroup = test.MakeRequirement(func(data test.Data, t *testing.T) (ret bool, mess string) {
	ret = true
	mess = "cgroup is enabled"
	(&test.GenericCommand{}).
		WithT(t).
		WithBinary(testutil.GetTarget()).
		WithArgs("info", "--format", "{{ json . }}").
		Run(&test.Expected{
			Output: func(stdout string, info string, t *testing.T) {
				var dinf dockercompat.Info
				err := json.Unmarshal([]byte(stdout), &dinf)
				assert.NilError(t, err, "failed to parse docker info")
				switch dinf.CgroupDriver {
				case "none", "":
					ret = false
					mess = "cgroup is none"
				}
			},
		})

	return ret, mess
})
