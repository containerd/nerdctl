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

	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func Setup() *test.Case {
	test.CustomCommand(&nerdctlSetup{})
	return &test.Case{
		Description: "root",
	}
}

var DockerConfig test.ConfigKey = "DockerConfig"
var Namespace test.ConfigKey = "Namespace"
var NerdctlToml test.ConfigKey = "NerdctlToml"
var HostsDir test.ConfigKey = "HostsDir"
var DataRoot test.ConfigKey = "DataRoot"
var Debug test.ConfigKey = "Debug"

type nerdctlSetup struct {
}

func (ns *nerdctlSetup) OnInitialize(testCase *test.Case, t *testing.T) test.Command {
	var err error
	var binary string
	trgt := GetTarget()
	switch trgt {
	case targetNerdctl:
		binary, err = exec.LookPath(trgt)
		if err != nil {
			t.Fatalf("unable to find binary %q: %v", trgt, err)
		}
	case TargetDocker:
		binary, err = exec.LookPath(trgt)
		if err != nil {
			t.Fatalf("unable to find binary %q: %v", trgt, err)
		}
		if err = exec.Command("docker", "compose", "version").Run(); err != nil {
			t.Fatalf("docker does not support compose: %v", err)
		}
	default:
		t.Fatalf("unknown target %q", GetTarget())
	}

	baseCommand := &nerdCommand{}
	baseCommand.WithBinary(binary)
	baseCommand.WithTempDir(testCase.Data.TempDir())
	baseCommand.WithEnv(testCase.Env)
	baseCommand.WithT(t)

	baseCommand.EnvBlackList = []string{
		"LS_COLORS",
		"DOCKER_CONFIG",
		"CONTAINERD_SNAPSHOTTER",
		"NERDCTL_TOML",
		"CONTAINERD_ADDRESS",
		"CNI_PATH",
		"NETCONFPATH",
		"NERDCTL_EXPERIMENTAL",
		"NERDCTL_HOST_GATEWAY_IP",
	}

	return baseCommand
}

func (ns *nerdctlSetup) OnPostRequirements(testCase *test.Case, t *testing.T, com test.Command) {
	// Ambient requirements, bail out now if these do not match
	if environmentHasIPv6() && testCase.Data.ReadConfig(ipv6) != only {
		t.Skip("runner skips non-IPv6 compatible tests in the IPv6 environment")
	}

	if environmentHasKubernetes() && testCase.Data.ReadConfig(kubernetes) != only {
		t.Skip("runner skips non-Kubernetes compatible tests in the Kubernetes environment")
	}

	if environmentIsForFlaky() && testCase.Data.ReadConfig(flaky) != only {
		t.Skip("runner skips non-flaky tests in the flaky environment")
	}

	data := testCase.Data
	if data.ReadConfig(mode) == modePrivate {
		// For docker, we do disable parallel since there is no namespace where we can isolate
		if GetTarget() == TargetDocker {
			testCase.NoParallel = true
		}
	}

	// Map the config in
	cc := com.(*nerdCommand)
	cc.DockerConfig = string(data.ReadConfig(DockerConfig))
	cc.Namespace = string(data.ReadConfig(Namespace))
	if cc.Namespace == "" {
		cc.Namespace = defaultNamespace
	}
	cc.NerdctlToml = string(data.ReadConfig(NerdctlToml))
	cc.HostsDir = string(data.ReadConfig(HostsDir))
	cc.DataRoot = string(data.ReadConfig(DataRoot))
	if data.ReadConfig(Debug) != "" {
		cc.Debug = true
	}
	// Save the namespace information into system - some tests do want it
	data.Sink(test.SystemKey(Namespace), test.SystemValue(cc.Namespace))
}

func (ns *nerdctlSetup) OnPostSetup(testCase *test.Case, t *testing.T, com test.Command) {
	// Some setup routines MAY alter config (specifically HostsDir, NerdctlToml, DataRoot)
	data := testCase.Data
	cc := com.(*nerdCommand)
	cc.NerdctlToml = string(data.ReadConfig(NerdctlToml))
	cc.HostsDir = string(data.ReadConfig(HostsDir))
	cc.DataRoot = string(data.ReadConfig(DataRoot))
}
