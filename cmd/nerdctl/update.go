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
	"errors"
	"fmt"
	"runtime"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/pkg/cri/util"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/typeurl"
	"github.com/docker/go-units"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type updateResourceOptions struct {
	CPUPeriod          uint64
	CPUQuota           int64
	CPUShares          uint64
	MemoryLimitInBytes int64
	MemoryReservation  int64
	MemorySwapInBytes  int64
	CpusetCpus         string
	CpusetMems         string
	PidsLimit          int64
	BlkioWeight        uint16
}

func newUpdateCommand() *cobra.Command {
	var updateCommand = &cobra.Command{
		Use:               "update [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Update one or more running containers",
		RunE:              updateAction,
		ValidArgsFunction: updateShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	updateCommand.Flags().SetInterspersed(false)
	setUpdateFlags(updateCommand)
	return updateCommand
}

func setUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().Float64("cpus", 0.0, "Number of CPUs")
	cmd.Flags().Uint64("cpu-period", 0, "Limit CPU CFS (Completely Fair Scheduler) period")
	cmd.Flags().Int64("cpu-quota", -1, "Limit CPU CFS (Completely Fair Scheduler) quota")
	cmd.Flags().Uint64("cpu-shares", 0, "CPU shares (relative weight)")
	cmd.Flags().StringP("memory", "m", "", "Memory limit")
	cmd.Flags().String("memory-reservation", "", "Memory soft limit")
	cmd.Flags().String("memory-swap", "", "Swap limit equal to memory plus swap: '-1' to enable unlimited swap")
	cmd.Flags().String("kernel-memory", "", "Kernel memory limit (deprecated)")
	cmd.Flags().String("cpuset-cpus", "", "CPUs in which to allow execution (0-3, 0,1)")
	cmd.Flags().String("cpuset-mems", "", "MEMs in which to allow execution (0-3, 0,1)")
	cmd.Flags().Int64("pids-limit", -1, "Tune container pids limit (set -1 for unlimited)")
	cmd.Flags().Uint16("blkio-weight", 0, "Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)")
	cmd.Flags().String("restart", "no", `Restart policy to apply when a container exits (implemented values: "no"|"always|on-failure:n|unless-stopped")`)
	cmd.RegisterFlagCompletionFunc("restart", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"no", "always", "on-failure", "unless-stopped"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func updateAction(cmd *cobra.Command, args []string) error {
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), namespace, address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getUpdateOption(cmd)
	if err != nil {
		return err
	}
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			err = updateContainer(ctx, client, found.Container.ID(), options, cmd)
			return err
		},
	}
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("no such container %s", req)
		}
	}
	return nil
}

func getUpdateOption(cmd *cobra.Command) (updateResourceOptions, error) {
	var options updateResourceOptions
	cpus, err := cmd.Flags().GetFloat64("cpus")
	if err != nil {
		return options, err
	}
	cpuPeriod, err := cmd.Flags().GetUint64("cpu-period")
	if err != nil {
		return options, err
	}
	cpuQuota, err := cmd.Flags().GetInt64("cpu-quota")
	if err != nil {
		return options, err
	}
	if cpuQuota != -1 || cpuPeriod != 0 {
		if cpus > 0.0 {
			return options, errors.New("cpus and quota/period should be used separately")
		}
	}
	if cpus > 0.0 {
		cpuPeriod = uint64(100000)
		cpuQuota = int64(cpus * 100000.0)
	}
	shares, err := cmd.Flags().GetUint64("cpu-shares")
	if err != nil {
		return options, err
	}
	memStr, err := cmd.Flags().GetString("memory")
	if err != nil {
		return options, err
	}
	memSwap, err := cmd.Flags().GetString("memory-swap")
	if err != nil {
		return options, err
	}
	var mem64 int64
	if memStr != "" {
		mem64, err = units.RAMInBytes(memStr)
		if err != nil {
			return options, fmt.Errorf("failed to parse memory bytes %q: %w", memStr, err)
		}
	}
	var memSwap64 int64
	if memSwap != "" {
		if memSwap == "-1" {
			memSwap64 = -1
		} else {
			memSwap64, err = units.RAMInBytes(memSwap)
			if err != nil {
				return options, fmt.Errorf("failed to parse memory-swap bytes %q: %w", memSwap, err)
			}
			if mem64 > 0 && memSwap64 > 0 && memSwap64 < mem64 {
				return options, fmt.Errorf("minimum memoryswap limit should be larger than memory limit, see usage")
			}
		}
	} else {
		memSwap64 = mem64 * 2
	}
	if memSwap64 == 0 {
		memSwap64 = mem64 * 2
	}
	memReserve, err := cmd.Flags().GetString("memory-reservation")
	if err != nil {
		return options, err
	}
	var memReserve64 int64
	if memReserve != "" {
		memReserve64, err = units.RAMInBytes(memReserve)
		if err != nil {
			return options, fmt.Errorf("failed to parse memory bytes %q: %w", memReserve, err)
		}
	}
	if mem64 > 0 && memReserve64 > 0 && mem64 < memReserve64 {
		return options, fmt.Errorf("minimum memory limit can not be less than memory reservation limit, see usage")
	}

	kernelMemStr, err := cmd.Flags().GetString("kernel-memory")
	if err != nil {
		return options, err
	}
	if kernelMemStr != "" && cmd.Flag("kernel-memory").Changed {
		logrus.Warnf("The --kernel-memory flag is no longer supported. This flag is a noop.")
	}
	cpuset, err := cmd.Flags().GetString("cpuset-cpus")
	if err != nil {
		return options, err
	}
	cpusetMems, err := cmd.Flags().GetString("cpuset-mems")
	if err != nil {
		return options, err
	}
	pidsLimit, err := cmd.Flags().GetInt64("pids-limit")
	if err != nil {
		return options, err
	}
	blkioWeight, err := cmd.Flags().GetUint16("blkio-weight")
	if err != nil {
		return options, err
	}
	cgroupManager, err := cmd.Flags().GetString("cgroup-manager")
	if err != nil {
		return options, err
	}
	if blkioWeight != 0 && !infoutil.BlockIOWeight(cgroupManager) {
		return options, fmt.Errorf("kernel support for cgroup blkio weight missing, weight discarded")
	}
	if blkioWeight > 0 && blkioWeight < 10 || blkioWeight > 1000 {
		return options, errors.New("range of blkio weight is from 10 to 1000")
	}

	if runtime.GOOS == "linux" {
		options = updateResourceOptions{
			CPUPeriod:          cpuPeriod,
			CPUQuota:           cpuQuota,
			CPUShares:          shares,
			CpusetCpus:         cpuset,
			CpusetMems:         cpusetMems,
			MemoryLimitInBytes: mem64,
			MemoryReservation:  memReserve64,
			MemorySwapInBytes:  memSwap64,
			PidsLimit:          pidsLimit,
			BlkioWeight:        blkioWeight,
		}
	}
	return options, nil
}

func updateContainer(ctx context.Context, client *containerd.Client, id string, opts updateResourceOptions, cmd *cobra.Command) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}
	cStatus := formatter.ContainerStatus(ctx, container)
	if cStatus == "pausing" {
		return fmt.Errorf("container %q is in pausing state", id)
	}
	spec, err := container.Spec(ctx)
	if err != nil {
		return err
	}

	oldSpec, err := copySpec(spec)
	if err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if spec.Linux == nil {
			spec.Linux = &runtimespec.Linux{}
		}
		if spec.Linux.Resources == nil {
			spec.Linux.Resources = &runtimespec.LinuxResources{}
		}
		if spec.Linux.Resources.BlockIO == nil {
			spec.Linux.Resources.BlockIO = &runtimespec.LinuxBlockIO{}
		}
		if cmd.Flags().Changed("blkio-weight") {
			if spec.Linux.Resources.BlockIO.Weight != &opts.BlkioWeight {
				spec.Linux.Resources.BlockIO.Weight = &opts.BlkioWeight
			}
		}
		if spec.Linux.Resources.CPU == nil {
			spec.Linux.Resources.CPU = &runtimespec.LinuxCPU{}
		}
		if cmd.Flags().Changed("cpu-shares") {
			if spec.Linux.Resources.CPU.Shares != &opts.CPUShares {
				spec.Linux.Resources.CPU.Shares = &opts.CPUShares
			}
		}
		if cmd.Flags().Changed("cpu-quota") {
			if spec.Linux.Resources.CPU.Quota != &opts.CPUQuota {
				spec.Linux.Resources.CPU.Quota = &opts.CPUQuota
			}
		}
		if cmd.Flags().Changed("cpu-period") {
			if spec.Linux.Resources.CPU.Period != &opts.CPUPeriod {
				spec.Linux.Resources.CPU.Period = &opts.CPUPeriod
			}
		}
		if cmd.Flags().Changed("cpus") {
			if spec.Linux.Resources.CPU.Cpus != opts.CpusetCpus {
				spec.Linux.Resources.CPU.Cpus = opts.CpusetCpus
			}
		}
		if cmd.Flags().Changed("cpuset-mems") {
			if spec.Linux.Resources.CPU.Mems != opts.CpusetMems {
				spec.Linux.Resources.CPU.Mems = opts.CpusetMems
			}
		}

		if cmd.Flags().Changed("cpuset-cpus") {
			if spec.Linux.Resources.CPU.Cpus != opts.CpusetCpus {
				spec.Linux.Resources.CPU.Cpus = opts.CpusetCpus
			}
		}
		if spec.Linux.Resources.Memory == nil {
			spec.Linux.Resources.Memory = &runtimespec.LinuxMemory{}
		}
		if cmd.Flags().Changed("memory") {
			if spec.Linux.Resources.Memory.Limit != &opts.MemoryLimitInBytes {
				spec.Linux.Resources.Memory.Limit = &opts.MemoryLimitInBytes
			}
			if spec.Linux.Resources.Memory.Swap != &opts.MemorySwapInBytes {
				spec.Linux.Resources.Memory.Swap = &opts.MemorySwapInBytes
			}
		}
		if cmd.Flags().Changed("memory-reservation") {
			if spec.Linux.Resources.Memory.Reservation != &opts.MemoryReservation {
				spec.Linux.Resources.Memory.Reservation = &opts.MemoryReservation
			}
		}
		if spec.Linux.Resources.Pids == nil {
			spec.Linux.Resources.Pids = &runtimespec.LinuxPids{}
		}
		if cmd.Flags().Changed("pids-limit") {
			if spec.Linux.Resources.Pids.Limit != opts.PidsLimit {
				spec.Linux.Resources.Pids.Limit = opts.PidsLimit
			}
		}
	}

	if err := updateContainerSpec(ctx, container, spec); err != nil {
		log.G(ctx).WithError(err).Errorf("Failed to update spec %+v for container %q", spec, id)
		// reset spec on error.
		if err := updateContainerSpec(ctx, container, oldSpec); err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to update spec %+v for container %q", oldSpec, id)
		}
	}

	restart, err := cmd.Flags().GetString("restart")
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("restart") && restart != "" {
		if err := updateContainerRestartPolicyLabel(ctx, client, container, restart); err != nil {
			return err
		}
	}

	// If container is not running, only update spec is enough, new resource
	// limit will be applied when container start.
	if cStatus != "Up" {
		return nil
	}
	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			// Task exited already.
			return nil
		}
		return fmt.Errorf("failed to get task:%w", err)
	}
	if err := task.Update(ctx, containerd.WithResources(spec.Linux.Resources)); err != nil {
		return err
	}
	return nil
}

func updateContainerSpec(ctx context.Context, container containerd.Container, spec *runtimespec.Spec) error {
	if err := container.Update(ctx, func(ctx context.Context, client *containerd.Client, c *containers.Container) error {
		any, err := typeurl.MarshalAny(spec)
		if err != nil {
			return fmt.Errorf("failed to marshal spec %+v:%w", spec, err)
		}
		c.Spec = any
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update container spec:%w", err)
	}
	return nil
}

func copySpec(spec *runtimespec.Spec) (*runtimespec.Spec, error) {
	var copySpec runtimespec.Spec
	if err := util.DeepCopy(&copySpec, spec); err != nil {
		return nil, fmt.Errorf("failed to deep copy:%w", err)
	}
	return &copySpec, nil
}

func updateShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return shellCompleteContainerNames(cmd, nil)
}
