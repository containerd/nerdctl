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

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/bypass4netnsutil"
	"github.com/containerd/nerdctl/pkg/containerinspector"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/cap"
	"github.com/containerd/containerd/pkg/userns"
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

	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return nil, err
	}

	securityOpt, err := cmd.Flags().GetStringArray("security-opt")
	if err != nil {
		return nil, err
	}
	securityOptsMaps := strutil.ConvertKVStringsToMap(strutil.DedupeStrSlice(securityOpt))
	if secOpts, err := generateSecurityOpts(privileged, securityOptsMaps); err != nil {
		return nil, err
	} else {
		opts = append(opts, secOpts...)
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
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return nil, err
	}

	// FIXME : setPlatformOptions() is invoked by createContainer() and createContainer() already has client.
	client, ctx, cancel, err := newClientWithPlatform(cmd, platform)
	if err != nil {
		return nil, err
	}
	defer cancel()

	if pidNs != "" {
		opts, err = setPIDNamespace(ctx, opts, client, pidNs)
		if err != nil {
			return nil, err
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

	opts, err = setOOMScoreAdj(opts, cmd)
	if err != nil {
		return nil, err
	}

	return opts, nil
}

func setOOMScoreAdj(opts []oci.SpecOpts, cmd *cobra.Command) ([]oci.SpecOpts, error) {
	if !cmd.Flags().Changed("oom-score-adj") {
		return opts, nil
	}

	score, err := cmd.Flags().GetInt("oom-score-adj")
	if err != nil {
		return nil, err
	}
	// score=0 means literally zero, not "unchanged"

	if score < -1000 || score > 1000 {
		return nil, fmt.Errorf("invalid value %d, range for oom score adj is [-1000, 1000]", score)
	}

	opts = append(opts, withOOMScoreAdj(score))
	return opts, nil
}

func withOOMScoreAdj(score int) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Process.OOMScoreAdj = &score
		return nil
	}
}

func setPIDNamespace(ctx context.Context, opts []oci.SpecOpts, client *containerd.Client, pidNs string) ([]oci.SpecOpts, error) {
	pidNs = strings.ToLower(pidNs)

	switch pidNs {
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
		if rootlessutil.IsRootless() {
			opts = append(opts, withBindMountHostProcfs)
		}
	default: // container:<name|id> form
		parsedPidNs := strings.Split(pidNs, ":")
		if len(parsedPidNs) < 2 {
			return nil, fmt.Errorf("invalid pid namespace. Set --pid=[host|container:<name|id>]")
		}

		req := parsedPidNs[1] // container's id or name
		walker := &containerwalker.ContainerWalker{
			Client: client,
			OnFound: func(ctx context.Context, found containerwalker.Found) error {
				if found.MatchCount > 1 {
					return fmt.Errorf("ambiguous condition for containers")
				}

				nc, err := containerinspector.Inspect(ctx, found.Container)
				if err != nil {
					return err
				}

				if nc.Process.Status.Status != containerd.Running {
					return fmt.Errorf("shared container is not running")
				}

				ns := specs.LinuxNamespace{
					Type: specs.PIDNamespace,
					Path: fmt.Sprintf("/proc/%d/ns/pid", nc.Process.Pid),
				}
				opts = append(opts, oci.WithLinuxNamespace(ns))

				if userns.RunningInUserNS() {
					nsUser := specs.LinuxNamespace{
						Type: specs.UserNamespace,
						Path: fmt.Sprintf("/proc/%d/ns/user", nc.Process.Pid),
					}
					opts = append(opts, oci.WithLinuxNamespace(nsUser))
				}
				return nil
			},
		}

		matchCount, err := walker.Walk(ctx, req)
		if err != nil {
			return nil, err
		}
		if matchCount == 0 {
			return nil, fmt.Errorf("invalid pid namespace. Set --pid=[host|container:<name|id>]")
		}
	}

	return opts, nil
}
