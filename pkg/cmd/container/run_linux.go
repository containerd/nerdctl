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
	"fmt"
	"strings"

	"github.com/moby/sys/userns"
	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/bypass4netnsutil"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/ipcutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

// WithoutRunMount returns a SpecOpts that unmounts the default tmpfs on "/run"
func WithoutRunMount() func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
	return oci.WithoutRunMount
}

func setPlatformOptions(ctx context.Context, client *containerd.Client, id, uts string, internalLabels *internalLabels, options types.ContainerCreateOptions) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts
	opts = append(opts,
		oci.WithDefaultUnixDevices,
		WithoutRunMount(), // unmount default tmpfs on "/run": https://github.com/containerd/nerdctl/issues/157)
	)

	opts = append(opts,
		oci.WithMounts([]specs.Mount{
			{Type: "cgroup", Source: "cgroup", Destination: "/sys/fs/cgroup", Options: []string{"ro", "nosuid", "noexec", "nodev"}},
		}))

	cgOpts, err := generateCgroupOpts(id, options)
	if err != nil {
		return nil, err
	}
	opts = append(opts, cgOpts...)

	annotations := strutil.ConvertKVStringsToMap(options.Annotations)

	capOpts, err := generateCapOpts(
		strutil.DedupeStrSlice(options.CapAdd),
		strutil.DedupeStrSlice(options.CapDrop))
	if err != nil {
		return nil, err
	}
	opts = append(opts, capOpts...)
	securityOptsMaps := strutil.ConvertKVStringsToMap(strutil.DedupeStrSlice(options.SecurityOpt))
	secOpts, err := generateSecurityOpts(options.Privileged, securityOptsMaps)
	if err != nil {
		return nil, err
	}
	opts = append(opts, secOpts...)

	b4nnOpts, err := bypass4netnsutil.GenerateBypass4netnsOpts(securityOptsMaps, annotations, id)
	if err != nil {
		return nil, err
	}
	opts = append(opts, b4nnOpts...)

	ulimitOpts, err := generateUlimitsOpts(options.Ulimit)
	if err != nil {
		return nil, err
	}

	// If without any ulimitOpts, we need to reset the default value from spec
	// which has 1024 as file limit. Make this behavior same as containerd/cri.
	if len(ulimitOpts) == 0 {
		ulimitOpts = append(ulimitOpts, withRlimits(nil))
	}

	opts = append(opts, ulimitOpts...)
	if options.Sysctl != nil {
		opts = append(opts, WithSysctls(strutil.ConvertKVStringsToMap(options.Sysctl)))
	}
	gpuOpt, err := parseGPUOpts(options.GPUs)
	if err != nil {
		return nil, err
	}
	opts = append(opts, gpuOpt...)

	if options.RDTClass != "" {
		opts = append(opts, oci.WithRdt(options.RDTClass, "", ""))
	}

	nsOpts, err := generateNamespaceOpts(ctx, client, uts, internalLabels, options)
	if err != nil {
		return nil, err
	}
	opts = append(opts, nsOpts...)

	opts, err = setOOMScoreAdj(opts, options.OomScoreAdjChanged, options.OomScoreAdj)
	if err != nil {
		return nil, err
	}

	return opts, nil
}

// generateNamespaceOpts help to validate the namespace options exposed via run and return the correct opts.
func generateNamespaceOpts(
	ctx context.Context,
	client *containerd.Client,
	uts string,
	internalLabels *internalLabels,
	options types.ContainerCreateOptions,
) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts

	switch uts {
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.UTSNamespace))
	case "":
		// Default, do nothing. Every container gets its own UTS ns by default.
	default:
		return nil, fmt.Errorf("unknown uts value. valid value(s) are 'host', got: %q", uts)
	}

	stateDir := internalLabels.stateDir
	ipcOpts, ipcLabel, err := generateIPCOpts(ctx, client, options.IPC, options.ShmSize, stateDir)
	if err != nil {
		return nil, err
	}
	internalLabels.ipc = ipcLabel
	opts = append(opts, ipcOpts...)

	pidOpts, pidLabel, err := generatePIDOpts(ctx, client, options.Pid)
	if err != nil {
		return nil, err
	}
	internalLabels.pidContainer = pidLabel
	opts = append(opts, pidOpts...)

	return opts, nil
}

func generateIPCOpts(ctx context.Context, client *containerd.Client, ipcFlag string, shmSize string, stateDir string) ([]oci.SpecOpts, string, error) {
	ipcFlag = strings.ToLower(ipcFlag)

	ipc, err := ipcutil.DetectFlags(ctx, client, stateDir, ipcFlag, shmSize)
	if err != nil {
		return nil, "", err
	}
	ipcLabel, err := ipcutil.EncodeIPCLabel(ipc)
	if err != nil {
		return nil, "", err
	}
	opts, err := ipcutil.GenerateIPCOpts(ctx, ipc, client)
	if err != nil {
		return nil, "", err
	}

	return opts, ipcLabel, nil
}

func generatePIDOpts(ctx context.Context, client *containerd.Client, pid string) ([]oci.SpecOpts, string, error) {
	opts := make([]oci.SpecOpts, 0)
	pid = strings.ToLower(pid)
	var pidInternalLabel string

	switch pid {
	case "":
		// do nothing
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
		if rootlessutil.IsRootless() {
			opts = append(opts, containerutil.WithBindMountHostProcfs)
		}
	default: // container:<id|name>
		parsed := strings.Split(pid, ":")
		if len(parsed) < 2 || parsed[0] != "container" {
			return nil, "", fmt.Errorf("invalid pid namespace. Set --pid=[host|container:<name|id>")
		}

		containerName := parsed[1]
		walker := &containerwalker.ContainerWalker{
			Client: client,
			OnFound: func(ctx context.Context, found containerwalker.Found) error {
				if found.MatchCount > 1 {
					return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
				}

				o, err := containerutil.GenerateSharingPIDOpts(ctx, found.Container)
				if err != nil {
					return err
				}
				opts = append(opts, o...)
				pidInternalLabel = found.Container.ID()

				return nil
			},
		}
		matchedCount, err := walker.Walk(ctx, containerName)
		if err != nil {
			return nil, "", err
		}
		if matchedCount < 1 {
			return nil, "", fmt.Errorf("no such container: %s", containerName)
		}
	}

	return opts, pidInternalLabel, nil
}

func setOOMScoreAdj(opts []oci.SpecOpts, oomScoreAdjChanged bool, oomScoreAdj int) ([]oci.SpecOpts, error) {
	if !oomScoreAdjChanged {
		return opts, nil
	}
	// score=0 means literally zero, not "unchanged"
	if oomScoreAdj < -1000 || oomScoreAdj > 1000 {
		return nil, fmt.Errorf("invalid value %d, range for oom score adj is [-1000, 1000]", oomScoreAdj)
	}

	if userns.RunningInUserNS() {
		// > The value of /proc/<pid>/oom_score_adj may be reduced no lower than the last value set by a CAP_SYS_RESOURCE process.
		// > To reduce the value any lower requires CAP_SYS_RESOURCE.
		// https://github.com/torvalds/linux/blob/v6.0/Documentation/filesystems/proc.rst#31-procpidoom_adj--procpidoom_score_adj--adjust-the-oom-killer-score
		//
		// The minimum=100 is from `/proc/$(pgrep -u $(id -u) systemd)/oom_score_adj`
		// (FIXME: find a more robust way to get the current minimum value)
		const minimum = 100
		if oomScoreAdj < minimum {
			log.L.Warnf("Limiting oom_score_adj (%d -> %d)", oomScoreAdj, minimum)
			oomScoreAdj = minimum
		}
	}

	opts = append(opts, withOOMScoreAdj(oomScoreAdj))
	return opts, nil
}

func withOOMScoreAdj(score int) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Process.OOMScoreAdj = &score
		return nil
	}
}
