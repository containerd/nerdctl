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
	"strings"

	"github.com/containerd/nerdctl/pkg/bypass4netnsutil"
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

	labelsMap, err := readKVStringsMapfFromLabel(cmd)
	if err != nil {
		return nil, err
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

	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return nil, err
	}
	if privileged {
		opts = append(opts, privilegedOpts...)
	}

	b4nnOpts, err := bypass4netnsutil.GenerateBypass4netnsOpts(securityOptsMaps, labelsMap, id)
	if err != nil {
		return nil, err
	} else {
		opts = append(opts, b4nnOpts...)
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
			return nil, fmt.Errorf("invalid pid namespace. Set --pid=host to enable host pid namespace")
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

	ipc, err := cmd.Flags().GetString("ipc")
	if err != nil {
		return nil, err
	}
	// if nothing is specified, or if private, default to normal behavior
	if ipc == "host" {
		opts = append(opts, oci.WithHostNamespace(specs.IPCNamespace))
		opts = append(opts, withBindMountHostIPC)
	} else if ipc != "" && ipc != "private" {
		return nil, fmt.Errorf("error: %v", "invalid ipc value, supported values are 'private' or 'host'")
	}

	return opts, nil
}
