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

	runcoptions "github.com/containerd/containerd/api/types/runc/options"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/containerd/v2/plugins"
	"github.com/containerd/log"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func generateRuntimeCOpts(cgroupManager, runtimeStr string) ([]containerd.NewContainerOpts, error) {
	runtime := plugins.RuntimeRuncV2
	var (
		runcOpts    runcoptions.Options
		runtimeOpts interface{} = &runcOpts
	)
	if cgroupManager == "systemd" {
		runcOpts.SystemdCgroup = true
	}
	if runtimeStr != "" {
		if strings.HasPrefix(runtimeStr, "io.containerd.") || runtimeStr == "wtf.sbk.runj.v1" {
			runtime = runtimeStr
			if !strings.HasPrefix(runtimeStr, "io.containerd.runc.") {
				if cgroupManager == "systemd" {
					log.L.Warnf("cannot set cgroup manager to %q for runtime %q", cgroupManager, runtimeStr)
				}
				runtimeOpts = nil
			}
		} else {
			// runtimeStr is a runc binary
			runcOpts.BinaryName = runtimeStr
		}
	}
	o := containerd.WithRuntime(runtime, runtimeOpts)
	return []containerd.NewContainerOpts{o}, nil
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
