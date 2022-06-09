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

package main

import (
	"context"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/plugin"
	runcoptions "github.com/containerd/containerd/runtime/v2/runc/options"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func generateRuntimeCOpts(cmd *cobra.Command) ([]containerd.NewContainerOpts, error) {
	runtime := plugin.RuntimeRuncV2
	var (
		runcOpts    runcoptions.Options
		runtimeOpts interface{} = &runcOpts
	)
	cgm, err := cmd.Flags().GetString("cgroup-manager")
	if err != nil {
		return nil, err
	}
	if cgm == "systemd" {
		runcOpts.SystemdCgroup = true
	}
	runtimeStr, err := cmd.Flags().GetString("runtime")
	if err != nil {
		return nil, err
	}
	if runtimeStr != "" {
		if strings.HasPrefix(runtimeStr, "io.containerd.") || runtimeStr == "wtf.sbk.runj.v1" {
			runtime = runtimeStr
			if !strings.HasPrefix(runtimeStr, "io.containerd.runc.") {
				if cgm == "systemd" {
					logrus.Warnf("cannot set cgroup manager to %q for runtime %q", cgm, runtimeStr)
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
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *runtimespec.Spec) error {
		if s.Linux == nil {
			s.Linux = &runtimespec.Linux{}
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
