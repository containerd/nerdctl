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
	"fmt"
	"runtime"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/spf13/cobra"
)

func newCreateCommand() *cobra.Command {
	shortHelp := "Create a new container. Optionally specify \"ipfs://\" or \"ipns://\" scheme to pull image from IPFS."
	longHelp := shortHelp
	switch runtime.GOOS {
	case "windows":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl create` is experimental on Windows and currently broken (https://github.com/containerd/nerdctl/issues/28)"
	case "freebsd":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl create` is experimental on FreeBSD and currently requires `--net=none` (https://github.com/containerd/nerdctl/blob/main/docs/freebsd.md)"
	}
	var createCommand = &cobra.Command{
		Use:               "create [flags] IMAGE [COMMAND] [ARG...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              createAction,
		ValidArgsFunction: runShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	createCommand.Flags().SetInterspersed(false)
	setCreateFlags(createCommand)
	return createCommand
}

func processContainerCreateOptions(cmd *cobra.Command) (opt types.ContainerCreateOptions, err error) {
	opt.Stdout = cmd.OutOrStdout()
	opt.Stderr = cmd.ErrOrStderr()
	opt.GOptions, err = processRootCmdFlags(cmd)
	if err != nil {
		return
	}

	opt.NerdctlCmd, opt.NerdctlArgs = globalFlags(cmd)

	// #region for basic flags
	// The command `container start` doesn't support the flag `--interactive`. Set the default value of `opt.Interactive` false.
	opt.Interactive = false
	opt.TTY, err = cmd.Flags().GetBool("tty")
	if err != nil {
		return
	}
	// The nerdctl create command similar to nerdctl run -d except the container is never started.
	// So we keep the default value of `opt.Detach` true.
	opt.Detach = true
	opt.Restart, err = cmd.Flags().GetString("restart")
	if err != nil {
		return
	}
	opt.Rm, err = cmd.Flags().GetBool("rm")
	if err != nil {
		return
	}
	opt.Pull, err = cmd.Flags().GetString("pull")
	if err != nil {
		return
	}
	opt.Pid, err = cmd.Flags().GetString("pid")
	if err != nil {
		return
	}
	opt.StopSignal, err = cmd.Flags().GetString("stop-signal")
	if err != nil {
		return
	}
	opt.StopTimeout, err = cmd.Flags().GetInt("stop-timeout")
	if err != nil {
		return
	}
	// #endregion

	// #region for platform flags
	opt.Platform, err = cmd.Flags().GetString("platform")
	if err != nil {
		return
	}
	// #endregion

	// #region for init process flags
	opt.InitProcessFlag, err = cmd.Flags().GetBool("init")
	if err != nil {
		return
	}
	if opt.InitProcessFlag || cmd.Flags().Changed("init-binary") {
		var initBinary string
		initBinary, err = cmd.Flags().GetString("init-binary")
		if err != nil {
			return
		}
		opt.InitBinary = &initBinary
	}
	// #endregion

	// #region for isolation flags
	opt.Isolation, err = cmd.Flags().GetString("isolation")
	if err != nil {
		return
	}
	// #endregion

	// #region for resource flags
	opt.CPUs, err = cmd.Flags().GetFloat64("cpus")
	if err != nil {
		return
	}
	opt.CPUQuota, err = cmd.Flags().GetInt64("cpu-quota")
	if err != nil {
		return
	}
	opt.CPUPeriod, err = cmd.Flags().GetUint64("cpu-period")
	if err != nil {
		return
	}
	opt.CPUShares, err = cmd.Flags().GetUint64("cpu-shares")
	if err != nil {
		return
	}
	opt.CPUSetCPUs, err = cmd.Flags().GetString("cpuset-cpus")
	if err != nil {
		return
	}
	opt.CPUSetMems, err = cmd.Flags().GetString("cpuset-mems")
	if err != nil {
		return
	}
	opt.Memory, err = cmd.Flags().GetString("memory")
	if err != nil {
		return
	}
	opt.MemoryReservationChanged = cmd.Flags().Changed("memory-reservation")
	opt.MemoryReservation, err = cmd.Flags().GetString("memory-reservation")
	if err != nil {
		return
	}
	opt.MemorySwap, err = cmd.Flags().GetString("memory-swap")
	if err != nil {
		return
	}
	opt.MemorySwappiness64Changed = cmd.Flags().Changed("memory-swappiness")
	opt.MemorySwappiness64, err = cmd.Flags().GetInt64("memory-swappiness")
	if err != nil {
		return
	}
	opt.KernelMemoryChanged = cmd.Flag("kernel-memory").Changed
	opt.KernelMemory, err = cmd.Flags().GetString("kernel-memory")
	if err != nil {
		return
	}
	opt.OomKillDisable, err = cmd.Flags().GetBool("oom-kill-disable")
	if err != nil {
		return
	}
	opt.OomScoreAdjChanged = cmd.Flags().Changed("oom-score-adj")
	opt.OomScoreAdj, err = cmd.Flags().GetInt("oom-score-adj")
	if err != nil {
		return
	}
	opt.PidsLimit, err = cmd.Flags().GetInt64("pids-limit")
	if err != nil {
		return
	}
	opt.CgroupConf, err = cmd.Flags().GetStringSlice("cgroup-conf")
	if err != nil {
		return
	}
	opt.BlkioWeight, err = cmd.Flags().GetUint16("blkio-weight")
	if err != nil {
		return
	}
	opt.Cgroupns, err = cmd.Flags().GetString("cgroupns")
	if err != nil {
		return
	}
	opt.CgroupParent, err = cmd.Flags().GetString("cgroup-parent")
	if err != nil {
		return
	}
	opt.Device, err = cmd.Flags().GetStringSlice("device")
	if err != nil {
		return
	}
	// #endregion

	// #region for intel RDT flags
	opt.RDTClass, err = cmd.Flags().GetString("rdt-class")
	if err != nil {
		return
	}
	// #endregion

	// #region for user flags
	// If user is set we will attempt to start container with that user (must be present on the host)
	// Otherwise we will inherit permissions from the user that the containerd process is running as
	opt.User, err = cmd.Flags().GetString("user")
	if err != nil {
		return
	}
	opt.Umask = ""
	if cmd.Flags().Changed("umask") {
		opt.Umask, err = cmd.Flags().GetString("umask")
		if err != nil {
			return
		}
	}
	opt.GroupAdd, err = cmd.Flags().GetStringSlice("group-add")
	if err != nil {
		return
	}
	// #endregion

	// #region for security flags
	opt.SecurityOpt, err = cmd.Flags().GetStringArray("security-opt")
	if err != nil {
		return
	}
	opt.CapAdd, err = cmd.Flags().GetStringSlice("cap-add")
	if err != nil {
		return
	}
	opt.CapDrop, err = cmd.Flags().GetStringSlice("cap-drop")
	if err != nil {
		return
	}
	opt.Privileged, err = cmd.Flags().GetBool("privileged")
	if err != nil {
		return
	}
	// #endregion

	// #region for runtime flags
	opt.Runtime, err = cmd.Flags().GetString("runtime")
	if err != nil {
		return
	}
	opt.Sysctl, err = cmd.Flags().GetStringArray("sysctl")
	if err != nil {
		return
	}
	// #endregion

	// #region for volume flags
	opt.Volume, err = cmd.Flags().GetStringArray("volume")
	if err != nil {
		return
	}
	// tmpfs needs to be StringArray, not StringSlice, to prevent "/foo:size=64m,exec" from being split to {"/foo:size=64m", "exec"}
	opt.Tmpfs, err = cmd.Flags().GetStringArray("tmpfs")
	if err != nil {
		return
	}
	opt.Mount, err = cmd.Flags().GetStringArray("mount")
	if err != nil {
		return
	}
	opt.VolumesFrom, err = cmd.Flags().GetStringArray("volumes-from")
	if err != nil {
		return
	}
	// #endregion

	// #region for rootfs flags
	opt.ReadOnly, err = cmd.Flags().GetBool("read-only")
	if err != nil {
		return
	}
	opt.Rootfs, err = cmd.Flags().GetBool("rootfs")
	if err != nil {
		return
	}
	// #endregion

	// #region for env flags
	opt.EntrypointChanged = cmd.Flags().Changed("entrypoint")
	opt.Entrypoint, err = cmd.Flags().GetStringArray("entrypoint")
	if err != nil {
		return
	}
	opt.Workdir, err = cmd.Flags().GetString("workdir")
	if err != nil {
		return
	}
	opt.Env, err = cmd.Flags().GetStringArray("env")
	if err != nil {
		return
	}
	opt.EnvFile, err = cmd.Flags().GetStringSlice("env-file")
	if err != nil {
		return
	}
	// #endregion

	// #region for metadata flags
	opt.NameChanged = cmd.Flags().Changed("name")
	opt.Name, err = cmd.Flags().GetString("name")
	if err != nil {
		return
	}
	opt.Label, err = cmd.Flags().GetStringArray("label")
	if err != nil {
		return
	}
	opt.LabelFile, err = cmd.Flags().GetStringSlice("label-file")
	if err != nil {
		return
	}
	opt.CidFile, err = cmd.Flags().GetString("cidfile")
	if err != nil {
		return
	}
	opt.PidFile = ""
	if cmd.Flags().Changed("pidfile") {
		opt.PidFile, err = cmd.Flags().GetString("pidfile")
		if err != nil {
			return
		}
	}
	// #endregion

	// #region for logging flags
	// json-file is the built-in and default log driver for nerdctl
	opt.LogDriver, err = cmd.Flags().GetString("log-driver")
	if err != nil {
		return
	}
	opt.LogOpt, err = cmd.Flags().GetStringArray("log-opt")
	if err != nil {
		return
	}
	// #endregion

	// #region for shared memory flags
	opt.IPC, err = cmd.Flags().GetString("ipc")
	if err != nil {
		return
	}
	opt.ShmSize, err = cmd.Flags().GetString("shm-size")
	if err != nil {
		return
	}
	// #endregion

	// #region for gpu flags
	opt.GPUs, err = cmd.Flags().GetStringArray("gpus")
	if err != nil {
		return
	}
	// #endregion

	// #region for ulimit flags
	opt.Ulimit, err = cmd.Flags().GetStringSlice("ulimit")
	if err != nil {
		return
	}
	// #endregion

	// #region for ipfs flags
	opt.IPFSAddress, err = cmd.Flags().GetString("ipfs-address")
	if err != nil {
		return
	}
	// #endregion

	// #region for image pull and verify options
	imageVerifyOpt, err := processImageVerifyOptions(cmd)
	if err != nil {
		return
	}
	opt.ImagePullOpt = types.ImagePullOptions{
		GOptions:      opt.GOptions,
		VerifyOptions: imageVerifyOpt,
		IPFSAddress:   opt.IPFSAddress,
		Stdout:        opt.Stdout,
		Stderr:        opt.Stderr,
	}
	// #endregion

	return opt, nil
}

func createAction(cmd *cobra.Command, args []string) error {
	createOpt, err := processContainerCreateOptions(cmd)
	if err != nil {
		return err
	}

	if (createOpt.Platform == "windows" || createOpt.Platform == "freebsd") && !createOpt.GOptions.Experimental {
		return fmt.Errorf("%s requires experimental mode to be enabled", createOpt.Platform)
	}
	client, ctx, cancel, err := clientutil.NewClientWithPlatform(cmd.Context(), createOpt.GOptions.Namespace, createOpt.GOptions.Address, createOpt.Platform)
	if err != nil {
		return err
	}
	defer cancel()

	netFlags, err := loadNetworkFlags(cmd)
	if err != nil {
		return fmt.Errorf("failed to load networking flags: %s", err)
	}

	netManager, err := containerutil.NewNetworkingOptionsManager(createOpt.GOptions, netFlags, client)
	if err != nil {
		return err
	}

	c, gc, err := container.Create(ctx, client, args, netManager, createOpt)
	if err != nil {
		if gc != nil {
			gc()
		}
		return err
	}
	// defer setting `nerdctl/error` label in case of error
	defer func() {
		if err != nil {
			containerutil.UpdateErrorLabel(ctx, c, err)
		}
	}()

	fmt.Fprintln(createOpt.Stdout, c.ID())
	return nil
}
