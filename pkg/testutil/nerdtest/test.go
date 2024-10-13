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
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

var DockerConfig test.ConfigKey = "DockerConfig"
var Namespace test.ConfigKey = "Namespace"
var NerdctlToml test.ConfigKey = "NerdctlToml"
var HostsDir test.ConfigKey = "HostsDir"
var DataRoot test.ConfigKey = "DataRoot"
var Debug test.ConfigKey = "Debug"

func Setup() *test.Case {
	test.Customize(&nerdctlSetup{})
	return &test.Case{
		Env: map[string]string{},
	}
}

type nerdctlSetup struct {
}

func (ns *nerdctlSetup) CustomCommand(testCase *test.Case, t *testing.T) test.CustomizableCommand {
	return newNerdCommand(testCase.Config, t)
}

func (ns *nerdctlSetup) AmbientRequirements(testCase *test.Case, t *testing.T) {
	// Ambient requirements, bail out now if these do not match
	if environmentHasIPv6() && testCase.Config.Read(ipv6) != only {
		t.Skip("runner skips non-IPv6 compatible tests in the IPv6 environment")
	}

	if environmentHasKubernetes() && testCase.Config.Read(kubernetes) != only {
		t.Skip("runner skips non-Kubernetes compatible tests in the Kubernetes environment")
	}

	if environmentIsForFlaky() && testCase.Config.Read(flaky) != only {
		t.Skip("runner skips non-flaky tests in the flaky environment")
	}

	if getTarget() == targetDocker && testCase.Config.Read(modePrivate) == enabled {
		// For docker, we do disable parallel since there is no namespace where we can isolate
		testCase.NoParallel = true
	}

	// We do not want private to get inherited by subtests, as we want them to be in the same namespace set here
	testCase.Config.Write(modePrivate, "")
}
