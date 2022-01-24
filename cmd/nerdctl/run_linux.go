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
	"fmt"
	"path"
	"strings"

	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/cap"
	"github.com/spf13/cobra"
)

func WithoutRunMount() func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
	return oci.WithoutRunMount
}

func capShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{}
	for _, c := range cap.Known() {
		// "CAP_SYS_ADMIN" -> "sys_admin"
		s := strings.ToLower(strings.TrimPrefix(c, "CAP_"))
		candidates = append(candidates, s)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func runShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return shellCompleteImageNames(cmd)
	} else {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func setPlatformOptions(opts []oci.SpecOpts, cmd *cobra.Command, id string) ([]oci.SpecOpts, error) {
	opts = append(opts,
		oci.WithDefaultUnixDevices,
		WithoutRunMount(), // unmount default tmpfs on "/run": https://github.com/containerd/nerdctl/issues/157)
	)

	opts = append(opts,
		oci.WithMounts([]specs.Mount{
			{Type: "cgroup", Source: "cgroup", Destination: "/sys/fs/cgroup", Options: []string{"ro", "nosuid", "noexec", "nodev"}},
		}))

	if cgOpts, err := generateCgroupOpts(cmd, id); err != nil {
		return nil, err
	} else {
		opts = append(opts, cgOpts...)
	}

	securityOpt, err := cmd.Flags().GetStringArray("security-opt")
	if err != nil {
		return nil, err
	}
	securityOptsMaps := strutil.ConvertKVStringsToMap(strutil.DedupeStrSlice(securityOpt))
	if secOpts, err := generateSecurityOpts(securityOptsMaps); err != nil {
		return nil, err
	} else {
		opts = append(opts, secOpts...)
	}

	capAdd, err := cmd.Flags().GetStringSlice("cap-add")
	if err != nil {
		return nil, err
	}
	capDrop, err := cmd.Flags().GetStringSlice("cap-drop")
	if err != nil {
		return nil, err
	}
	if capOpts, err := generateCapOpts(
		strutil.DedupeStrSlice(capAdd),
		strutil.DedupeStrSlice(capDrop)); err != nil {
		return nil, err
	} else {
		opts = append(opts, capOpts...)
	}

	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return nil, err
	}
	if privileged {
		opts = append(opts, privilegedOpts...)
	}

	shmSize, err := cmd.Flags().GetString("shm-size")
	if err != nil {
		return nil, err
	}
	if len(shmSize) > 0 {
		shmBytes, err := units.RAMInBytes(shmSize)
		if err != nil {
			return nil, err
		}
		opts = append(opts, oci.WithDevShmSize(shmBytes/1024))
	}

	pidNs, err := cmd.Flags().GetString("pid")
	if err != nil {
		return nil, err
	}
	pidNs = strings.ToLower(pidNs)
	if pidNs != "" {
		if pidNs != "host" {
			return nil, fmt.Errorf("Invalid pid namespace. Set --pid=host to enable host pid namespace.")
		} else {
			opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
			if rootlessutil.IsRootless() {
				opts = append(opts, withBindMountHostProcfs)
			}
		}
	}

	ulimitOpts, err := generateUlimitsOpts(cmd)
	if err != nil {
		return nil, err
	}
	opts = append(opts, ulimitOpts...)

	sysctl, err := cmd.Flags().GetStringArray("sysctl")
	if err != nil {
		return nil, err
	}
	opts = append(opts, WithSysctls(strutil.ConvertKVStringsToMap(sysctl)))

	gpus, err := cmd.Flags().GetStringArray("gpus")
	if err != nil {
		return nil, err
	}
	gpuOpt, err := parseGPUOpts(gpus)
	if err != nil {
		return nil, err
	}
	opts = append(opts, gpuOpt...)

	if rdtClass, err := cmd.Flags().GetString("rdt-class"); err != nil {
		return nil, err
	} else if rdtClass != "" {
		opts = append(opts, oci.WithRdt(rdtClass, "", ""))
	}

	return opts, nil
}

// withBindMountHostProcfs replaces procfs mount with rbind.
// Required for --pid=host on rootless.
//
// https://github.com/moby/moby/pull/41893/files
// https://github.com/containers/podman/blob/v3.0.0-rc1/pkg/specgen/generate/oci.go#L248-L257
func withBindMountHostProcfs(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		if path.Clean(m.Destination) == "/proc" {
			newM := specs.Mount{
				Destination: "/proc",
				Type:        "bind",
				Source:      "/proc",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
	}

	// Remove ReadonlyPaths for /proc/*
	newROP := s.Linux.ReadonlyPaths[:0]
	for _, x := range s.Linux.ReadonlyPaths {
		x = path.Clean(x)
		if !strings.HasPrefix(x, "/proc/") {
			newROP = append(newROP, x)
		}
	}
	s.Linux.ReadonlyPaths = newROP
	return nil
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
