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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/cmd/image"
	"github.com/containerd/nerdctl/pkg/consoleutil"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/errutil"
	"github.com/containerd/nerdctl/pkg/flagutil"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/nerdctl/pkg/signalutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/taskutil"
	dockercliopts "github.com/docker/cli/opts"
	dockeropts "github.com/docker/docker/opts"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	tiniInitBinary = "tini"
)

func newRunCommand() *cobra.Command {
	shortHelp := "Run a command in a new container. Optionally specify \"ipfs://\" or \"ipns://\" scheme to pull image from IPFS."
	longHelp := shortHelp
	switch runtime.GOOS {
	case "windows":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl run` is experimental on Windows and currently broken (https://github.com/containerd/nerdctl/issues/28)"
	case "freebsd":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl run` is experimental on FreeBSD and currently requires `--net=none` (https://github.com/containerd/nerdctl/blob/main/docs/freebsd.md)"
	}
	var runCommand = &cobra.Command{
		Use:               "run [flags] IMAGE [COMMAND] [ARG...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              runAction,
		ValidArgsFunction: runShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	runCommand.Flags().SetInterspersed(false)
	setCreateFlags(runCommand)

	runCommand.Flags().BoolP("detach", "d", false, "Run container in background and print container ID")

	return runCommand
}

func setCreateFlags(cmd *cobra.Command) {

	// No "-h" alias for "--help", because "-h" for "--hostname".
	cmd.Flags().Bool("help", false, "show help")

	cmd.Flags().BoolP("tty", "t", false, "Allocate a pseudo-TTY")
	cmd.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")
	cmd.Flags().String("restart", "no", `Restart policy to apply when a container exits (implemented values: "no"|"always|on-failure:n|unless-stopped")`)
	cmd.RegisterFlagCompletionFunc("restart", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"no", "always", "on-failure", "unless-stopped"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().Bool("rm", false, "Automatically remove the container when it exits")
	cmd.Flags().String("pull", "missing", `Pull image before running ("always"|"missing"|"never")`)
	cmd.RegisterFlagCompletionFunc("pull", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"always", "missing", "never"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("stop-signal", "SIGTERM", "Signal to stop a container")
	cmd.Flags().Int("stop-timeout", 0, "Timeout (in seconds) to stop a container")

	// #region for init process
	cmd.Flags().Bool("init", false, "Run an init process inside the container, Default to use tini")
	cmd.Flags().String("init-binary", tiniInitBinary, "The custom binary to use as the init process")
	// #endregion

	// #region platform flags
	cmd.Flags().String("platform", "", "Set platform (e.g. \"amd64\", \"arm64\")") // not a slice, and there is no --all-platforms
	cmd.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	// #endregion

	// #region network flags
	// network (net) is defined as StringSlice, not StringArray, to allow specifying "--network=cni1,cni2"
	cmd.Flags().StringSlice("network", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none"|"container:<container>"|<CNI>)`)
	cmd.RegisterFlagCompletionFunc("network", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return shellCompleteNetworkNames(cmd, []string{})
	})
	cmd.Flags().StringSlice("net", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none"|<CNI>)`)
	cmd.RegisterFlagCompletionFunc("net", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return shellCompleteNetworkNames(cmd, []string{})
	})
	// dns is defined as StringSlice, not StringArray, to allow specifying "--dns=1.1.1.1,8.8.8.8" (compatible with Podman)
	cmd.Flags().StringSlice("dns", nil, "Set custom DNS servers")
	cmd.Flags().StringSlice("dns-search", nil, "Set custom DNS search domains")
	// We allow for both "--dns-opt" and "--dns-option", although the latter is the recommended way.
	cmd.Flags().StringSlice("dns-opt", nil, "Set DNS options")
	cmd.Flags().StringSlice("dns-option", nil, "Set DNS options")
	// publish is defined as StringSlice, not StringArray, to allow specifying "--publish=80:80,443:443" (compatible with Podman)
	cmd.Flags().StringSliceP("publish", "p", nil, "Publish a container's port(s) to the host")
	// FIXME: not support IPV6 yet
	cmd.Flags().String("ip", "", "IPv4 address to assign to the container")
	cmd.Flags().StringP("hostname", "h", "", "Container host name")
	cmd.Flags().String("mac-address", "", "MAC address to assign to the container")
	// #endregion

	cmd.Flags().String("ipc", "", `IPC namespace to use ("host"|"private")`)
	cmd.RegisterFlagCompletionFunc("ipc", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host", "private"}, cobra.ShellCompDirectiveNoFileComp
	})
	// #region cgroups, namespaces, and ulimits flags
	cmd.Flags().Float64("cpus", 0.0, "Number of CPUs")
	cmd.Flags().StringP("memory", "m", "", "Memory limit")
	cmd.Flags().String("memory-reservation", "", "Memory soft limit")
	cmd.Flags().String("memory-swap", "", "Swap limit equal to memory plus swap: '-1' to enable unlimited swap")
	cmd.Flags().Int64("memory-swappiness", -1, "Tune container memory swappiness (0 to 100) (default -1)")
	cmd.Flags().String("kernel-memory", "", "Kernel memory limit (deprecated)")
	cmd.Flags().Bool("oom-kill-disable", false, "Disable OOM Killer")
	cmd.Flags().Int("oom-score-adj", 0, "Tune container’s OOM preferences (-1000 to 1000, rootless: 100 to 1000)")
	cmd.Flags().String("pid", "", "PID namespace to use")
	cmd.Flags().String("uts", "", "UTS namespace to use")
	cmd.RegisterFlagCompletionFunc("pid", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().Int64("pids-limit", -1, "Tune container pids limit (set -1 for unlimited)")
	cmd.Flags().StringSlice("cgroup-conf", nil, "Configure cgroup v2 (key=value)")
	cmd.Flags().Uint16("blkio-weight", 0, "Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)")
	cmd.Flags().String("cgroupns", defaults.CgroupnsMode(), `Cgroup namespace to use, the default depends on the cgroup version ("host"|"private")`)
	cmd.Flags().String("cgroup-parent", "", "Optional parent cgroup for the container")
	cmd.RegisterFlagCompletionFunc("cgroupns", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host", "private"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("cpuset-cpus", "", "CPUs in which to allow execution (0-3, 0,1)")
	cmd.Flags().String("cpuset-mems", "", "MEMs in which to allow execution (0-3, 0,1)")
	cmd.Flags().Uint64("cpu-shares", 0, "CPU shares (relative weight)")
	cmd.Flags().Int64("cpu-quota", -1, "Limit CPU CFS (Completely Fair Scheduler) quota")
	cmd.Flags().Uint64("cpu-period", 0, "Limit CPU CFS (Completely Fair Scheduler) period")
	cmd.Flags().Int64("cpu-rt-runtime", -1, "Limit CPU RT (Realtime scheduling) runtime")
	cmd.Flags().Uint64("cpu-rt-period", 0, "Limit CPU RT (Realtime scheduling) period")
	// device is defined as StringSlice, not StringArray, to allow specifying "--device=DEV1,DEV2" (compatible with Podman)
	cmd.Flags().StringSlice("device", nil, "Add a host device to the container")
	// ulimit is defined as StringSlice, not StringArray, to allow specifying "--ulimit=ULIMIT1,ULIMIT2" (compatible with Podman)
	cmd.Flags().StringSlice("ulimit", nil, "Ulimit options")
	cmd.Flags().String("rdt-class", "", "Name of the RDT class (or CLOS) to associate the container with")
	// #endregion

	// user flags
	cmd.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	cmd.Flags().String("umask", "", "Set the umask inside the container. Defaults to 0022")
	cmd.Flags().StringSlice("group-add", []string{}, "Add additional groups to join")

	// #region security flags
	cmd.Flags().StringArray("security-opt", []string{}, "Security options")
	cmd.RegisterFlagCompletionFunc("security-opt", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"seccomp=", "seccomp=unconfined", "apparmor=", "apparmor=" + defaults.AppArmorProfileName, "apparmor=unconfined", "no-new-privileges", "privileged-without-host-devices"}, cobra.ShellCompDirectiveNoFileComp
	})
	// cap-add and cap-drop are defined as StringSlice, not StringArray, to allow specifying "--cap-add=CAP_SYS_ADMIN,CAP_NET_ADMIN" (compatible with Podman)
	cmd.Flags().StringSlice("cap-add", []string{}, "Add Linux capabilities")
	cmd.RegisterFlagCompletionFunc("cap-add", capShellComplete)
	cmd.Flags().StringSlice("cap-drop", []string{}, "Drop Linux capabilities")
	cmd.RegisterFlagCompletionFunc("cap-drop", capShellComplete)
	cmd.Flags().Bool("privileged", false, "Give extended privileges to this container")
	// #endregion

	// #region runtime flags
	cmd.Flags().String("runtime", defaults.Runtime, "Runtime to use for this container, e.g. \"crun\", or \"io.containerd.runsc.v1\"")
	// sysctl needs to be StringArray, not StringSlice, to prevent "foo=foo1,foo2" from being split to {"foo=foo1", "foo2"}
	cmd.Flags().StringArray("sysctl", nil, "Sysctl options")
	// gpus needs to be StringArray, not StringSlice, to prevent "capabilities=utility,device=DEV" from being split to {"capabilities=utility", "device=DEV"}
	cmd.Flags().StringArray("gpus", nil, "GPU devices to add to the container ('all' to pass all GPUs)")
	cmd.RegisterFlagCompletionFunc("gpus", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"all"}, cobra.ShellCompDirectiveNoFileComp
	})
	// #endregion

	// #region mount flags
	// volume needs to be StringArray, not StringSlice, to prevent "/foo:/foo:ro,Z" from being split to {"/foo:/foo:ro", "Z"}
	cmd.Flags().StringArrayP("volume", "v", nil, "Bind mount a volume")
	// tmpfs needs to be StringArray, not StringSlice, to prevent "/foo:size=64m,exec" from being split to {"/foo:size=64m", "exec"}
	cmd.Flags().StringArray("tmpfs", nil, "Mount a tmpfs directory")
	cmd.Flags().StringArray("mount", nil, "Attach a filesystem mount to the container")
	// #endregion

	// rootfs flags
	cmd.Flags().Bool("read-only", false, "Mount the container's root filesystem as read only")
	// rootfs flags (from Podman)
	cmd.Flags().Bool("rootfs", false, "The first argument is not an image but the rootfs to the exploded container")

	// #region env flags
	// entrypoint needs to be StringArray, not StringSlice, to prevent "FOO=foo1,foo2" from being split to {"FOO=foo1", "foo2"}
	// entrypoint StringArray is an internal implementation to support `nerdctl compose` entrypoint yaml filed with multiple strings
	// users are not expected to specify multiple --entrypoint flags manually.
	cmd.Flags().StringArray("entrypoint", nil, "Overwrite the default ENTRYPOINT of the image")
	cmd.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	// env needs to be StringArray, not StringSlice, to prevent "FOO=foo1,foo2" from being split to {"FOO=foo1", "foo2"}
	cmd.Flags().StringArrayP("env", "e", nil, "Set environment variables")
	// add-host is defined as StringSlice, not StringArray, to allow specifying "--add-host=HOST1:IP1,HOST2:IP2" (compatible with Podman)
	cmd.Flags().StringSlice("add-host", nil, "Add a custom host-to-IP mapping (host:ip)")
	// env-file is defined as StringSlice, not StringArray, to allow specifying "--env-file=FILE1,FILE2" (compatible with Podman)
	cmd.Flags().StringSlice("env-file", nil, "Set environment variables from file")

	// #region metadata flags
	cmd.Flags().String("name", "", "Assign a name to the container")
	// label needs to be StringArray, not StringSlice, to prevent "foo=foo1,foo2" from being split to {"foo=foo1", "foo2"}
	cmd.Flags().StringArrayP("label", "l", nil, "Set metadata on container")
	cmd.RegisterFlagCompletionFunc("label", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return labels.ShellCompletions, cobra.ShellCompDirectiveNoFileComp
	})

	// label-file is defined as StringSlice, not StringArray, to allow specifying "--env-file=FILE1,FILE2" (compatible with Podman)
	cmd.Flags().StringSlice("label-file", nil, "Set metadata on container from file")
	cmd.Flags().String("cidfile", "", "Write the container ID to the file")
	// #endregion

	// #region logging flags
	// log-opt needs to be StringArray, not StringSlice, to prevent "env=os,customer" from being split to {"env=os", "customer"}
	cmd.Flags().String("log-driver", "json-file", "Logging driver for the container. Default is json-file. It also supports logURI (eg: --log-driver binary://<path>)")
	cmd.RegisterFlagCompletionFunc("log-driver", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return logging.Drivers(), cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringArray("log-opt", nil, "Log driver options")
	// #endregion

	// shared memory flags
	cmd.Flags().String("shm-size", "", "Size of /dev/shm")
	cmd.Flags().String("pidfile", "", "file path to write the task's pid")

	// #region verify flags
	cmd.Flags().String("verify", "none", "Verify the image (none|cosign|notation)")
	cmd.RegisterFlagCompletionFunc("verify", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign", "notation"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("cosign-key", "", "Path to the public key file, KMS, URI or Kubernetes Secret for --verify=cosign")
	// #endregion

	cmd.Flags().String("ipfs-address", "", "multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)")

	cmd.Flags().String("isolation", "default", "Specify isolation technology for container. On Linux the only valid value is default. Windows options are host, process and hyperv with process isolation as the default")
	cmd.RegisterFlagCompletionFunc("isolation", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if runtime.GOOS == "windows" {
			return []string{"default", "host", "process", "hyperv"}, cobra.ShellCompDirectiveNoFileComp
		}
		return []string{"default"}, cobra.ShellCompDirectiveNoFileComp
	})

}

// runAction is heavily based on ctr implementation:
// https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
func runAction(cmd *cobra.Command, args []string) (err error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClientWithPlatform(cmd.Context(), globalOptions.Namespace, globalOptions.Address, platform)
	if err != nil {
		return err
	}
	defer cancel()

	flagD, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}
	flagI, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}
	flagT, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return err
	}
	rm, err := cmd.Flags().GetBool("rm")
	if err != nil {
		return err
	}

	if rm && flagD {
		return errors.New("flags -d and --rm cannot be specified together")
	}

	netFlags, err := loadNetworkFlags(cmd)
	if err != nil {
		return fmt.Errorf("failed to load networking flags: %s", err)
	}

	netManager, err := containerutil.NewNetworkingOptionsManager(globalOptions, netFlags)
	if err != nil {
		return err
	}

	c, gc, err := createContainer(ctx, cmd, client, netManager, globalOptions, args, platform, flagI, flagT, flagD)
	if err != nil {
		if gc != nil {
			defer gc()
		}
		return err
	}
	// defer setting `nerdctl/error` label in case of error
	defer func() {
		if err != nil {
			containerutil.UpdateErrorLabel(ctx, c, err)
		}
	}()

	id := c.ID()
	if rm && !flagD {
		defer func() {
			// NOTE: OCI hooks (which are used for CNI network setup/teardown on Linux)
			// are not currently supported on Windows, so we must explicitly call
			// network setup/cleanup from the main nerdctl executable.
			if runtime.GOOS == "windows" {
				if err := netManager.CleanupNetworking(ctx, c); err != nil {
					logrus.Warnf("failed to clean up container networking: %s", err)
				}
			}
			if err := container.RemoveContainer(ctx, c, globalOptions, true, true); err != nil {
				logrus.WithError(err).Warnf("failed to remove container %s", id)
			}
		}()
	}

	var con console.Console
	if flagT && !flagD {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	lab, err := c.Labels(ctx)
	if err != nil {
		return err
	}
	logURI := lab[labels.LogURI]
	task, err := taskutil.NewTask(ctx, client, c, false, flagI, flagT, flagD, con, logURI)
	if err != nil {
		return err
	}
	var statusC <-chan containerd.ExitStatus
	if !flagD {
		defer func() {
			if rm {
				if _, taskDeleteErr := task.Delete(ctx); taskDeleteErr != nil {
					logrus.Error(taskDeleteErr)
				}
			}
		}()
		statusC, err = task.Wait(ctx)
		if err != nil {
			return err
		}
	}

	if err := task.Start(ctx); err != nil {
		return err
	}

	if flagD {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", id)
		return nil
	}
	if flagT {
		if err := consoleutil.HandleConsoleResize(ctx, task, con); err != nil {
			logrus.WithError(err).Error("console resize")
		}
	} else {
		sigc := signalutil.ForwardAllSignals(ctx, task)
		defer signalutil.StopCatch(sigc)
	}
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return errutil.NewExitCoderErr(int(code))
	}
	return nil
}

// FIXME: split to smaller functions
func createContainer(ctx context.Context, cmd *cobra.Command, client *containerd.Client, netManager containerutil.NetworkOptionsManager, globalOptions types.GlobalCommandOptions, args []string, platform string, flagI, flagT, flagD bool) (containerd.Container, func(), error) {
	// simulate the behavior of double dash
	newArg := []string{}
	if len(args) >= 2 && args[1] == "--" {
		newArg = append(newArg, args[:1]...)
		newArg = append(newArg, args[2:]...)
		args = newArg
	}
	var internalLabels internalLabels
	internalLabels.platform = platform

	internalLabels.namespace = globalOptions.Namespace

	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		id    = idgen.GenerateID()
	)

	cidfile, err := cmd.Flags().GetString("cidfile")
	if err != nil {
		return nil, nil, err
	}
	if cidfile != "" {
		if err := writeCIDFile(cidfile, id); err != nil {
			return nil, nil, err
		}
	}
	dataStore, err := clientutil.DataStore(globalOptions.DataRoot, globalOptions.Address)
	if err != nil {
		return nil, nil, err
	}

	stateDir, err := containerutil.ContainerStateDirPath(globalOptions, dataStore, id)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, nil, err
	}
	internalLabels.stateDir = stateDir

	opts = append(opts,
		oci.WithDefaultSpec(),
	)

	platformOpts, err := setPlatformOptions(ctx, cmd, client, globalOptions, id, &internalLabels)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, platformOpts...)

	rootfsOpts, rootfsCOpts, ensuredImage, err := generateRootfsOpts(ctx, client, platform, cmd, globalOptions, args, id)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, rootfsOpts...)
	cOpts = append(cOpts, rootfsCOpts...)

	wd, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return nil, nil, err
	}
	if wd != "" {
		opts = append(opts, oci.WithProcessCwd(wd))
	}

	envFile, err := cmd.Flags().GetStringSlice("env-file")
	if err != nil {
		return nil, nil, err
	}
	env, err := cmd.Flags().GetStringArray("env")
	if err != nil {
		return nil, nil, err
	}
	envs, err := flagutil.MergeEnvFileAndOSEnv(envFile, env)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, oci.WithEnv(envs))

	if flagI {
		if flagD {
			return nil, nil, errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		opts = append(opts, oci.WithTTY)
	}

	mountOpts, anonVolumes, mountPoints, err := generateMountOpts(ctx, cmd, client, globalOptions, ensuredImage)
	if err != nil {
		return nil, nil, err
	}
	internalLabels.anonVolumes = anonVolumes
	internalLabels.mountPoints = mountPoints
	opts = append(opts, mountOpts...)

	// always set internalLabels.logURI
	// to support restart the container that run with "-it", like
	//
	// 1, nerdctl run --name demo -it imagename
	// 2, ctrl + c to stop demo container
	// 3, nerdctl start/restart demo
	var logURI string
	{
		// json-file is the built-in and default log driver for nerdctl
		logDriver, err := cmd.Flags().GetString("log-driver")
		if err != nil {
			return nil, nil, err
		}

		// check if log driver is a valid uri. If it is a valid uri and scheme is not
		if u, err := url.Parse(logDriver); err == nil && u.Scheme != "" {
			logURI = logDriver
		} else {
			logOptMap, err := parseKVStringsMapFromLogOpt(cmd, logDriver)
			if err != nil {
				return nil, nil, err
			}
			logDriverInst, err := logging.GetDriver(logDriver, logOptMap)
			if err != nil {
				return nil, nil, err
			}
			if err := logDriverInst.Init(dataStore, globalOptions.Namespace, id); err != nil {
				return nil, nil, err
			}
			logConfig := &logging.LogConfig{
				Driver: logDriver,
				Opts:   logOptMap,
			}
			logConfigB, err := json.Marshal(logConfig)
			if err != nil {
				return nil, nil, err
			}
			logConfigFilePath := logging.LogConfigFilePath(dataStore, globalOptions.Namespace, id)
			if err = os.WriteFile(logConfigFilePath, logConfigB, 0600); err != nil {
				return nil, nil, err
			}
			if lu, err := generateLogURI(dataStore); err != nil {
				return nil, nil, err
			} else if lu != nil {
				logrus.Debugf("generated log driver: %s", lu.String())

				logURI = lu.String()
			}
		}
	}
	internalLabels.logURI = logURI

	restartValue, err := cmd.Flags().GetString("restart")
	if err != nil {
		return nil, nil, err
	}
	restartOpts, err := generateRestartOpts(ctx, client, restartValue, logURI)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, restartOpts...)

	stopSignal, err := cmd.Flags().GetString("stop-signal")
	if err != nil {
		return nil, nil, err
	}
	stopTimeout, err := cmd.Flags().GetInt("stop-timeout")
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, withStop(stopSignal, stopTimeout, ensuredImage))

	err = netManager.VerifyNetworkOptions(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to verify networking settings: %s", err)
	}

	netOpts, netNewContainerOpts, err := netManager.ContainerNetworkingOpts(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate networking spec options: %s", err)
	}
	opts = append(opts, netOpts...)
	cOpts = append(cOpts, netNewContainerOpts...)

	netLabelOpts, err := netManager.InternalNetworkingOptionLabels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate internal networking labels: %s", err)
	}
	// TODO(aznashwan): more formal way to load net opts into internalLabels:
	internalLabels.hostname = netLabelOpts.Hostname
	internalLabels.ports = netLabelOpts.PortMappings
	internalLabels.ipAddress = netLabelOpts.IPAddress
	internalLabels.networks = netLabelOpts.NetworkSlice
	internalLabels.macAddress = netLabelOpts.MACAddress

	// NOTE: OCI hooks are currently not supported on Windows so we skip setting them altogether.
	// The OCI hooks we define (whose logic can be found in pkg/ocihook) primarily
	// perform network setup and teardown when using CNI networking.
	// On Windows, we are forced to set up and tear down the networking from within nerdctl.
	if runtime.GOOS != "windows" {
		hookOpt, err := withNerdctlOCIHook(cmd, id)
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, hookOpt)
	}

	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return nil, nil, err
	}
	uOpts, err := container.GenerateUserOpts(user)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, uOpts...)

	groups, err := cmd.Flags().GetStringSlice("group-add")
	if err != nil {
		return nil, nil, err
	}
	gOpts, err := container.GenerateGroupsOpts(groups)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, gOpts...)

	var umask string
	if cmd.Flags().Changed("umask") {
		umask, err = cmd.Flags().GetString("umask")
		if err != nil {
			return nil, nil, err
		}
	}
	umaskOpts, err := container.GenerateUmaskOpts(umask)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, umaskOpts...)

	rtCOpts, err := generateRuntimeCOpts(cmd, globalOptions)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, rtCOpts...)

	lCOpts, err := withContainerLabels(cmd)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, lCOpts...)

	var containerNameStore namestore.NameStore
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return nil, nil, err
	}
	if name == "" && !cmd.Flags().Changed("name") {
		// Automatically set the container name, unless `--name=""` was explicitly specified.
		var imageRef string
		if ensuredImage != nil {
			imageRef = ensuredImage.Ref
		}
		name = referenceutil.SuggestContainerName(imageRef, id)
	}
	if name != "" {
		containerNameStore, err = namestore.New(dataStore, globalOptions.Namespace)
		if err != nil {
			return nil, nil, err
		}
		if err := containerNameStore.Acquire(name, id); err != nil {
			return nil, nil, err
		}
	}
	internalLabels.name = name

	var pidFile string
	if cmd.Flags().Lookup("pidfile").Changed {
		pidFile, err = cmd.Flags().GetString("pidfile")
		if err != nil {
			return nil, nil, err
		}
	}
	internalLabels.pidFile = pidFile

	extraHosts, err := cmd.Flags().GetStringSlice("add-host")
	if err != nil {
		return nil, nil, err
	}
	extraHosts = strutil.DedupeStrSlice(extraHosts)
	for i, host := range extraHosts {
		if _, err := dockercliopts.ValidateExtraHost(host); err != nil {
			return nil, nil, err
		}
		parts := strings.SplitN(host, ":", 2)
		// If the IP Address is a string called "host-gateway", replace this value with the IP address stored
		// in the daemon level HostGateway IP config variable.
		if parts[1] == dockeropts.HostGatewayName {
			if globalOptions.HostGatewayIP == "" {
				return nil, nil, fmt.Errorf("unable to derive the IP value for host-gateway")
			}
			parts[1] = globalOptions.HostGatewayIP
			extraHosts[i] = fmt.Sprintf(`%s:%s`, parts[0], parts[1])
		}
	}
	internalLabels.extraHosts = extraHosts

	ilOpt, err := withInternalLabels(internalLabels)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, propagateContainerdLabelsToOCIAnnotations())

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)

	cOpts = append(cOpts, spec)

	container, containerErr := client.NewContainer(ctx, id, cOpts...)
	var netSetupErr error
	// NOTE: on non-Windows platforms, network setup is performed by OCI hooks.
	// Seeing as though Windows does not currently support OCI hooks, we must explicitly
	// perform network setup/teardown in the main nerdctl executable.
	if containerErr == nil && runtime.GOOS == "windows" {
		netSetupErr = netManager.SetupNetworking(ctx, id)
		logrus.WithError(netSetupErr).Warnf("networking setup error has occurred")
	}

	if containerErr != nil || netSetupErr != nil {
		gcContainer := func() {
			if containerErr == nil {
				netGcErr := netManager.CleanupNetworking(ctx, container)
				if netGcErr != nil {
					logrus.WithError(netGcErr).Warnf("failed to revert container %q networking settings", id)
				}
			}

			if rmErr := os.RemoveAll(stateDir); rmErr != nil {
				logrus.WithError(rmErr).Warnf("failed to remove container %q state dir %q", id, stateDir)
			}

			if name != "" {
				var errE error
				if containerNameStore, errE = namestore.New(dataStore, globalOptions.Namespace); errE != nil {
					logrus.WithError(errE).Warnf("failed to instantiate container name store during cleanup for container %q", id)
				}
				if errE = containerNameStore.Release(name, id); errE != nil {
					logrus.WithError(errE).Warnf("failed to release container name store for container %q (%s)", name, id)
				}
			}
		}

		returnedError := containerErr
		if netSetupErr != nil {
			returnedError = netSetupErr // mutually exclusive
		}
		return nil, gcContainer, returnedError
	}

	return container, nil, nil
}

// When refactor `nerdctl run`, this func should be removed and replaced by
// creating a `PullCommandOptions` directly from `RunCommandOptions`.
func processPullCommandFlagsInRun(cmd *cobra.Command) (types.ImagePullOptions, error) {
	imageVerifyOptions, err := processImageVerifyOptions(cmd)
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	ipfsAddressStr, err := cmd.Flags().GetString("ipfs-address")
	if err != nil {
		return types.ImagePullOptions{}, err
	}
	return types.ImagePullOptions{
		VerifyOptions: imageVerifyOptions,
		IPFSAddress:   ipfsAddressStr,
		Stdout:        cmd.OutOrStdout(),
		Stderr:        cmd.ErrOrStderr(),
	}, nil
}

func generateRootfsOpts(ctx context.Context, client *containerd.Client, platform string, cmd *cobra.Command, globalOptions types.GlobalCommandOptions, args []string, id string) ([]oci.SpecOpts, []containerd.NewContainerOpts, *imgutil.EnsuredImage, error) {
	var (
		ensured *imgutil.EnsuredImage
		err     error
	)
	imageless, err := cmd.Flags().GetBool("rootfs")
	if err != nil {
		return nil, nil, nil, err
	}
	if !imageless {
		pull, err := cmd.Flags().GetString("pull")
		if err != nil {
			return nil, nil, nil, err
		}
		var platformSS []string // len: 0 or 1
		if platform != "" {
			platformSS = append(platformSS, platform)
		}
		ocispecPlatforms, err := platformutil.NewOCISpecPlatformSlice(false, platformSS)
		if err != nil {
			return nil, nil, nil, err
		}
		rawRef := args[0]

		options, err := processPullCommandFlagsInRun(cmd)
		if err != nil {
			return nil, nil, nil, err
		}

		options.GOptions = globalOptions
		ensured, err = image.EnsureImage(ctx, client, rawRef, ocispecPlatforms, pull, nil, false, options)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
	)
	if !imageless {
		cOpts = append(cOpts,
			containerd.WithImage(ensured.Image),
			containerd.WithSnapshotter(ensured.Snapshotter),
			containerd.WithNewSnapshot(id, ensured.Image),
			containerd.WithImageStopSignal(ensured.Image, "SIGTERM"),
		)

		if len(ensured.ImageConfig.Env) == 0 {
			opts = append(opts, oci.WithDefaultPathEnv)
		}
		for ind, env := range ensured.ImageConfig.Env {
			if strings.HasPrefix(env, "PATH=") {
				break
			} else {
				if ind == len(ensured.ImageConfig.Env)-1 {
					opts = append(opts, oci.WithDefaultPathEnv)
				}
			}
		}
	} else {
		absRootfs, err := filepath.Abs(args[0])
		if err != nil {
			return nil, nil, nil, err
		}
		opts = append(opts, oci.WithRootFSPath(absRootfs), oci.WithDefaultPathEnv)
	}

	// NOTE: "--entrypoint" can be set to an empty string, see TestRunEntrypoint* in run_test.go .
	entrypoint, err := cmd.Flags().GetStringArray("entrypoint")
	if err != nil {
		return nil, nil, nil, err
	}

	if !imageless && !cmd.Flag("entrypoint").Changed {
		opts = append(opts, oci.WithImageConfigArgs(ensured.Image, args[1:]))
	} else {
		if !imageless {
			opts = append(opts, oci.WithImageConfig(ensured.Image))
		}
		var processArgs []string
		if len(entrypoint) != 0 {
			processArgs = append(processArgs, entrypoint...)
		}
		if len(args) > 1 {
			processArgs = append(processArgs, args[1:]...)
		}
		if len(processArgs) == 0 {
			// error message is from Podman
			return nil, nil, nil, errors.New("no command or entrypoint provided, and no CMD or ENTRYPOINT from image")
		}
		opts = append(opts, oci.WithProcessArgs(processArgs...))
	}

	initProcessFlag, err := cmd.Flags().GetBool("init")
	if err != nil {
		return nil, nil, nil, err
	}
	initBinary, err := cmd.Flags().GetString("init-binary")
	if err != nil {
		return nil, nil, nil, err
	}
	if cmd.Flags().Changed("init-binary") {
		initProcessFlag = true
	}
	if initProcessFlag {
		binaryPath, err := exec.LookPath(initBinary)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return nil, nil, nil, fmt.Errorf(`init binary %q not found`, initBinary)
			}
			return nil, nil, nil, err
		}
		inContainerPath := filepath.Join("/sbin", filepath.Base(initBinary))
		opts = append(opts, func(_ context.Context, _ oci.Client, _ *containers.Container, spec *oci.Spec) error {
			spec.Process.Args = append([]string{inContainerPath, "--"}, spec.Process.Args...)
			spec.Mounts = append([]specs.Mount{{
				Destination: inContainerPath,
				Type:        "bind",
				Source:      binaryPath,
				Options:     []string{"bind", "ro"},
			}}, spec.Mounts...)
			return nil
		})
	}

	readonly, err := cmd.Flags().GetBool("read-only")
	if err != nil {
		return nil, nil, nil, err
	}
	if readonly {
		opts = append(opts, oci.WithRootFSReadonly())
	}
	return opts, cOpts, ensured, nil
}

// withBindMountHostIPC replaces /dev/shm and /dev/mqueue  mount with rbind.
// Required for --ipc=host on rootless.
func withBindMountHostIPC(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		switch p := path.Clean(m.Destination); p {
		case "/dev/shm", "/dev/mqueue":
			s.Mounts[i] = specs.Mount{
				Destination: p,
				Type:        "bind",
				Source:      p,
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
		}
	}
	return nil
}

func generateLogURI(dataStore string) (*url.URL, error) {
	selfExe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := map[string]string{
		logging.MagicArgv1: dataStore,
	}

	return cio.LogURIGenerator("binary", selfExe, args)
}

func withNerdctlOCIHook(cmd *cobra.Command, id string) (oci.SpecOpts, error) {
	selfExe, f := globalFlags(cmd)
	args := append([]string{selfExe}, append(f, "internal", "oci-hook")...)
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Hooks == nil {
			s.Hooks = &specs.Hooks{}
		}
		crArgs := append(args, "createRuntime")
		s.Hooks.CreateRuntime = append(s.Hooks.CreateRuntime, specs.Hook{
			Path: selfExe,
			Args: crArgs,
			Env:  os.Environ(),
		})
		argsCopy := append([]string(nil), args...)
		psArgs := append(argsCopy, "postStop")
		s.Hooks.Poststop = append(s.Hooks.Poststop, specs.Hook{
			Path: selfExe,
			Args: psArgs,
			Env:  os.Environ(),
		})
		return nil
	}, nil
}

func withContainerLabels(cmd *cobra.Command) ([]containerd.NewContainerOpts, error) {
	labelMap, err := readKVStringsMapfFromLabel(cmd)
	if err != nil {
		return nil, err
	}
	o := containerd.WithAdditionalContainerLabels(labelMap)
	return []containerd.NewContainerOpts{o}, nil
}

func readKVStringsMapfFromLabel(cmd *cobra.Command) (map[string]string, error) {
	labelsMap, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return nil, err
	}
	labelsMap = strutil.DedupeStrSlice(labelsMap)
	labelsFilePath, err := cmd.Flags().GetStringSlice("label-file")
	if err != nil {
		return nil, err
	}
	labelsFilePath = strutil.DedupeStrSlice(labelsFilePath)
	labels, err := dockercliopts.ReadKVStrings(labelsFilePath, labelsMap)
	if err != nil {
		return nil, err
	}

	return strutil.ConvertKVStringsToMap(labels), nil
}

// parseKVStringsMapFromLogOpt parse log options KV entries and convert to Map
func parseKVStringsMapFromLogOpt(cmd *cobra.Command, logDriver string) (map[string]string, error) {
	logOptArray, err := cmd.Flags().GetStringArray("log-opt")
	if err != nil {
		return nil, err
	}
	logOptArray = strutil.DedupeStrSlice(logOptArray)
	logOptMap := strutil.ConvertKVStringsToMap(logOptArray)
	if logDriver == "json-file" {
		if _, ok := logOptMap[logging.MaxSize]; !ok {
			delete(logOptMap, logging.MaxFile)
		}
	}
	if err := logging.ValidateLogOpts(logDriver, logOptMap); err != nil {
		return nil, err
	}
	return logOptMap, nil
}

func withStop(stopSignal string, stopTimeout int, ensuredImage *imgutil.EnsuredImage) containerd.NewContainerOpts {
	return func(ctx context.Context, _ *containerd.Client, c *containers.Container) error {
		if c.Labels == nil {
			c.Labels = make(map[string]string)
		}
		var err error
		if ensuredImage != nil {
			stopSignal, err = containerd.GetOCIStopSignal(ctx, ensuredImage.Image, stopSignal)
			if err != nil {
				return err
			}
		}
		c.Labels[containerd.StopSignalLabel] = stopSignal
		if stopTimeout != 0 {
			c.Labels[labels.StopTimout] = strconv.Itoa(stopTimeout)
		}
		return nil
	}
}

type internalLabels struct {
	// labels from cmd options
	namespace  string
	platform   string
	extraHosts []string
	pidFile    string
	// labels from cmd options or automatically set
	name     string
	hostname string
	// automatically generated
	stateDir string
	// network
	networks   []string
	ipAddress  string
	ports      []gocni.PortMapping
	macAddress string
	// volumn
	mountPoints []*mountutil.Processed
	anonVolumes []string
	// pid namespace
	pidContainer string
	// log
	logURI string
}

func withInternalLabels(internalLabels internalLabels) (containerd.NewContainerOpts, error) {
	m := make(map[string]string)
	m[labels.Namespace] = internalLabels.namespace
	if internalLabels.name != "" {
		m[labels.Name] = internalLabels.name
	}
	m[labels.Hostname] = internalLabels.hostname
	extraHostsJSON, err := json.Marshal(internalLabels.extraHosts)
	if err != nil {
		return nil, err
	}
	m[labels.ExtraHosts] = string(extraHostsJSON)
	m[labels.StateDir] = internalLabels.stateDir
	networksJSON, err := json.Marshal(internalLabels.networks)
	if err != nil {
		return nil, err
	}
	m[labels.Networks] = string(networksJSON)
	if len(internalLabels.ports) > 0 {
		portsJSON, err := json.Marshal(internalLabels.ports)
		if err != nil {
			return nil, err
		}
		m[labels.Ports] = string(portsJSON)
	}
	if internalLabels.logURI != "" {
		m[labels.LogURI] = internalLabels.logURI
	}
	if len(internalLabels.anonVolumes) > 0 {
		anonVolumeJSON, err := json.Marshal(internalLabels.anonVolumes)
		if err != nil {
			return nil, err
		}
		m[labels.AnonymousVolumes] = string(anonVolumeJSON)
	}

	if internalLabels.pidFile != "" {
		m[labels.PIDFile] = internalLabels.pidFile
	}

	if internalLabels.ipAddress != "" {
		m[labels.IPAddress] = internalLabels.ipAddress
	}

	m[labels.Platform], err = platformutil.NormalizeString(internalLabels.platform)
	if err != nil {
		return nil, err
	}

	if len(internalLabels.mountPoints) > 0 {
		mounts := dockercompatMounts(internalLabels.mountPoints)
		mountPointsJSON, err := json.Marshal(mounts)
		if err != nil {
			return nil, err
		}
		m[labels.Mounts] = string(mountPointsJSON)
	}

	if internalLabels.macAddress != "" {
		m[labels.MACAddress] = internalLabels.macAddress
	}

	if internalLabels.pidContainer != "" {
		m[labels.PIDContainer] = internalLabels.pidContainer
	}

	return containerd.WithAdditionalContainerLabels(m), nil
}

func dockercompatMounts(mountPoints []*mountutil.Processed) []dockercompat.MountPoint {
	reuslt := make([]dockercompat.MountPoint, len(mountPoints))
	for i := range mountPoints {
		mp := mountPoints[i]
		reuslt[i] = dockercompat.MountPoint{
			Type:        mp.Type,
			Name:        mp.Name,
			Source:      mp.Mount.Source,
			Destination: mp.Mount.Destination,
			Driver:      "",
			Mode:        mp.Mode,
		}

		// it's a anonymous volume
		if mp.AnonymousVolume != "" {
			reuslt[i].Name = mp.AnonymousVolume
		}

		// volume only support local driver
		if mp.Type == "volume" {
			reuslt[i].Driver = "local"
		}
	}
	return reuslt
}

func propagateContainerdLabelsToOCIAnnotations() oci.SpecOpts {
	return func(ctx context.Context, oc oci.Client, c *containers.Container, s *oci.Spec) error {
		return oci.WithAnnotations(c.Labels)(ctx, oc, c, s)
	}
}

func writeCIDFile(path, id string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("container ID file found, make sure the other container isn't running or delete %s", path)
	} else if errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if err != nil {
			return fmt.Errorf("failed to create the container ID file: %s", err)
		}
		if _, err := f.WriteString(id); err != nil {
			return err
		}
		return nil
	} else {
		return err
	}
}
