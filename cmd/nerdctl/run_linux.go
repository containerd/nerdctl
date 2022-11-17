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
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
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
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func setPlatformOptions(ctx context.Context, opts []oci.SpecOpts, cmd *cobra.Command, client *containerd.Client, id string) ([]oci.SpecOpts, error) {
	opts = append(opts,
		oci.WithDefaultUnixDevices,
		WithoutRunMount(), // unmount default tmpfs on "/run": https://github.com/containerd/nerdctl/issues/157)
	)

	opts = append(opts,
		oci.WithMounts([]specs.Mount{
			{Type: "cgroup", Source: "cgroup", Destination: "/sys/fs/cgroup", Options: []string{"ro", "nosuid", "noexec", "nodev"}},
		}))

	cgOpts, err := generateCgroupOpts(cmd, id)
	if err != nil {
		return nil, err
	}
	opts = append(opts, cgOpts...)

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
	capOpts, err := generateCapOpts(
		strutil.DedupeStrSlice(capAdd),
		strutil.DedupeStrSlice(capDrop))
	if err != nil {
		return nil, err
	}
	opts = append(opts, capOpts...)

	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return nil, err
	}

	securityOpt, err := cmd.Flags().GetStringArray("security-opt")
	if err != nil {
		return nil, err
	}
	securityOptsMaps := strutil.ConvertKVStringsToMap(strutil.DedupeStrSlice(securityOpt))
	secOpts, err := generateSecurityOpts(privileged, securityOptsMaps)
	if err != nil {
		return nil, err
	}
	opts = append(opts, secOpts...)

	b4nnOpts, err := bypass4netnsutil.GenerateBypass4netnsOpts(securityOptsMaps, labelsMap, id)
	if err != nil {
		return nil, err
	}
	opts = append(opts, b4nnOpts...)

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

	pid, err := cmd.Flags().GetString("pid")
	if err != nil {
		return nil, err
	}
	pidOpts, err := generatePIDOpts(ctx, client, pid)
	if err != nil {
		return nil, err
	}
	opts = append(opts, pidOpts...)

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

func setPlatformContainerOptions(ctx context.Context, cOpts []containerd.NewContainerOpts, cmd *cobra.Command, client *containerd.Client, id string) ([]containerd.NewContainerOpts, error) {
	pid, err := cmd.Flags().GetString("pid")
	if err != nil {
		return nil, err
	}

	pidCOpts, err := generatePIDCOpts(ctx, client, pid)
	if err != nil {
		return nil, err
	}
	cOpts = append(cOpts, pidCOpts...)

	return cOpts, nil
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

func generatePIDOpts(ctx context.Context, client *containerd.Client, pid string) ([]oci.SpecOpts, error) {
	opts := make([]oci.SpecOpts, 0)
	pid = strings.ToLower(pid)

	switch pid {
	case "":
		// do nothing
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
		if rootlessutil.IsRootless() {
			opts = append(opts, withBindMountHostProcfs)
		}
	default: // container:<id|name>
		parsed := strings.Split(pid, ":")
		if len(parsed) < 2 || parsed[0] != "container" {
			return nil, fmt.Errorf("invalid pid namespace. Set --pid=[host|container:<name|id>")
		}

		containerName := parsed[1]
		walker := &containerwalker.ContainerWalker{
			Client: client,
			OnFound: func(ctx context.Context, found containerwalker.Found) error {
				if found.MatchCount > 1 {
					return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
				}

				o, err := generateSharingPIDOpts(ctx, found.Container)
				if err != nil {
					return err
				}
				opts = append(opts, o...)

				return nil
			},
		}
		matchedCount, err := walker.Walk(ctx, containerName)
		if err != nil {
			return nil, err
		}
		if matchedCount < 1 {
			return nil, fmt.Errorf("no such container: %s", containerName)
		}
	}

	return opts, nil
}

func generatePIDCOpts(ctx context.Context, client *containerd.Client, pid string) ([]containerd.NewContainerOpts, error) {
	pid = strings.ToLower(pid)

	cOpts := make([]containerd.NewContainerOpts, 0)
	parsed := strings.Split(pid, ":")
	if len(parsed) < 2 || parsed[0] != "container" {
		// no need to save pid options
		return cOpts, nil
	}

	containerName := parsed[1]
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			cOpts = append(cOpts, containerd.WithAdditionalContainerLabels(map[string]string{
				labels.PIDContainer: containerName,
			}))
			return nil
		},
	}
	matchedCount, err := walker.Walk(ctx, containerName)
	if err != nil {
		return nil, err
	}
	if matchedCount < 1 {
		return nil, fmt.Errorf("no such container: %s", containerName)
	}
	return cOpts, nil
}
