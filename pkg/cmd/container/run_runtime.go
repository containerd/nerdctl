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
	"context"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	runtimeoptions "github.com/containerd/containerd/pkg/runtimeoptions/v1"
	"github.com/containerd/containerd/plugin"
	runcoptions "github.com/containerd/containerd/runtime/v2/runc/options"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func generateRuntimeOpts(options types.ContainerCreateOptions) containerd.NewContainerOpts {
	runtimeOpts := generateRuntimeCOpts(options.GOptions.CgroupManager, options.Runtime)

	if options.Runtime == "wtf.sbk.runj.v1" {
		runtimeOpts = generateRunjOpts()
	}

	return runtimeOpts
}

func generateRunjOpts() containerd.NewContainerOpts {
	return containerd.WithRuntime("wtf.sbk.runj.v1", &runtimeoptions.Options{
		ConfigPath: "/etc/nerdctl/runj.ext.json",
	})
}

func generateRuntimeCOpts(cgroupManager, runtimeStr string) containerd.NewContainerOpts {
	runtime := plugin.RuntimeRuncV2
	var runcOpts runcoptions.Options

	if cgroupManager == "systemd" {
		runcOpts.SystemdCgroup = true
	}
	if runtimeStr != "" {
		if strings.HasPrefix(runtimeStr, "io.containerd.") {
			runtime = runtimeStr
			if !strings.HasPrefix(runtimeStr, "io.containerd.runc.") {
				if cgroupManager == "systemd" {
					logrus.Warnf("cannot set cgroup manager to %q for runtime %q", cgroupManager, runtimeStr)
				}
				return nil
			}
		} else {
			// runtimeStr is a runc binary
			runcOpts.BinaryName = runtimeStr
		}
	}

	return containerd.WithRuntime(runtime, &runcOpts)
}

// WithSysctls sets the provided sysctls onto the spec
func WithSysctls(sysctls map[string]string) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *specs.Spec) error {
		if s.Linux == nil {
			s.Linux = &specs.Linux{}
		}
		if s.Linux.Sysctl == nil {
			s.Linux.Sysctl = make(map[string]string)
		}
		for k, v := range sysctls {
			s.Linux.Sysctl[k] = v
		}
		return nil
	}
}
