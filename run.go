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
	"bufio"
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
	"strings"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/defaults"
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
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/docker/cli/opts"
	dopts "github.com/docker/cli/opts"
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
		longHelp += "WARNING: `nerdctl run` is experimental on FreeBSD and currently requires `--net=none` (https://github.com/containerd/nerdctl/blob/master/docs/freebsd.md)"
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

	cmd.Flags().BoolP("tty", "t", false, "(Currently -t needs to correspond to -i)")
	cmd.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")
	cmd.Flags().String("restart", "no", `Restart policy to apply when a container exits (implemented values: "no"|"always")`)
	cmd.RegisterFlagCompletionFunc("restart", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"no", "always", "on-failure", "unless-stopped"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().Bool("rm", false, "Automatically remove the container when it exits")
	cmd.Flags().String("pull", "missing", `Pull image before running ("always"|"missing"|"never")`)
	cmd.RegisterFlagCompletionFunc("pull", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"always", "missing", "never"}, cobra.ShellCompDirectiveNoFileComp
	})

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
	cmd.Flags().StringSlice("network", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none"|<CNI>)`)
	cmd.RegisterFlagCompletionFunc("network", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return shellCompleteNetworkNames(cmd, []string{})
	})
	cmd.Flags().StringSlice("net", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none"|<CNI>)`)
	cmd.RegisterFlagCompletionFunc("net", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return shellCompleteNetworkNames(cmd, []string{})
	})
	// dns is defined as StringSlice, not StringArray, to allow specifying "--dns=1.1.1.1,8.8.8.8" (compatible with Podman)
	cmd.Flags().StringSlice("dns", nil, "Set custom DNS servers")
	// publish is defined as StringSlice, not StringArray, to allow specifying "--publish=80:80,443:443" (compatible with Podman)
	cmd.Flags().StringSliceP("publish", "p", nil, "Publish a container's port(s) to the host")
	// FIXME: not support IPV6 yet
	cmd.Flags().String("ip", "", "IPv4 address to assign to the container")
	cmd.Flags().StringP("hostname", "h", "", "Container host name")
	// #endregion

	cmd.Flags().String("ipc", "", `IPC namespace to use ("host"|"private")`)
	cmd.RegisterFlagCompletionFunc("ipc", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host", "private"}, cobra.ShellCompDirectiveNoFileComp
	})
	// #region cgroups, namespaces, and ulimits flags
	cmd.Flags().Float64("cpus", 0.0, "Number of CPUs")
	cmd.Flags().StringP("memory", "m", "", "Memory limit")
	cmd.Flags().String("pid", "", "PID namespace to use")
	cmd.RegisterFlagCompletionFunc("pid", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().Int64("pids-limit", -1, "Tune container pids limit (set -1 for unlimited)")
	cmd.Flags().StringSlice("cgroup-conf", nil, "Configure cgroup v2 (key=value)")
	cmd.Flags().Uint16("blkio-weight", 0, "Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)")
	cmd.Flags().String("cgroupns", defaults.CgroupnsMode(), `Cgroup namespace to use, the default depends on the cgroup version ("host"|"private")`)
	cmd.RegisterFlagCompletionFunc("cgroupns", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host", "private"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("cpuset-cpus", "", "CPUs in which to allow execution (0-3, 0,1)")
	cmd.Flags().String("cpuset-mems", "", "MEMs in which to allow execution (0-3, 0,1)")
	cmd.Flags().Uint64("cpu-shares", 0, "CPU shares (relative weight)")
	cmd.Flags().Int64("cpu-quota", -1, "Limit CPU CFS (Completely Fair Scheduler) quota")
	cmd.Flags().Uint64("cpu-period", 0, "Limit CPU CFS (Completely Fair Scheduler) period")
	// device is defined as StringSlice, not StringArray, to allow specifying "--device=DEV1,DEV2" (compatible with Podman)
	cmd.Flags().StringSlice("device", nil, "Add a host device to the container")
	// ulimit is defined as StringSlice, not StringArray, to allow specifying "--ulimit=ULIMIT1,ULIMIT2" (compatible with Podman)
	cmd.Flags().StringSlice("ulimit", nil, "Ulimit options")
	cmd.Flags().String("rdt-class", "", "Name of the RDT class (or CLOS) to associate the container with")
	// #endregion

	// user flags
	cmd.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")

	// #region security flags
	cmd.Flags().StringArray("security-opt", []string{}, "Security options")
	cmd.RegisterFlagCompletionFunc("security-opt", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"seccomp=", "seccomp=unconfined", "apparmor=", "apparmor=" + defaults.AppArmorProfileName, "apparmor=unconfined", "no-new-privileges"}, cobra.ShellCompDirectiveNoFileComp
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
	cmd.Flags().String("log-driver", "json-file", "Logging driver for the container")
	cmd.RegisterFlagCompletionFunc("log-driver", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json-file", "journald"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringArray("log-opt", nil, "Log driver options")
	// #endregion

	// shared memory flags
	cmd.Flags().String("shm-size", "", "Size of /dev/shm")
	cmd.Flags().String("pidfile", "", "file path to write the task's pid")

	// #region verify flags
	cmd.Flags().String("verify", "none", "Verify the image (none|cosign)")
	cmd.RegisterFlagCompletionFunc("verify", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("cosign-key", "", "Path to the public key file, KMS, URI or Kubernetes Secret for --verify=cosign")
	// #endregion
}

// runAction is heavily based on ctr implementation:
// https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
//
// FIXME: split to smaller functions
func runAction(cmd *cobra.Command, args []string) error {
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := newClientWithPlatform(cmd, platform)
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
	container, dataStore, containerNameStore, err := createContainer(cmd, ctx, client, args, platform, flagI, flagT, flagD)
	if err != nil {
		return err
	}

	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	logURI := lab[labels.LogURI]

	id := container.ID()
	rm, err := cmd.Flags().GetBool("rm")
	if err != nil {
		return err
	}
	if rm {
		if flagD {
			return errors.New("flag -d and --rm cannot be specified together")
		}
		defer func() {
			const removeAnonVolumes = true
			ns := lab[labels.Namespace]
			stateDir := lab[labels.StateDir]
			if removeErr := removeContainer(cmd, ctx, container, ns, id, true, dataStore, stateDir, containerNameStore, removeAnonVolumes); removeErr != nil {
				logrus.WithError(removeErr).Warnf("failed to remove container %s", id)
			}
		}()
	}

	var con console.Console
	if flagT {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	task, err := taskutil.NewTask(ctx, client, container, flagI, flagT, flagD, con, logURI)
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
		if err := tasks.HandleConsoleResize(ctx, task, con); err != nil {
			logrus.WithError(err).Error("console resize")
		}
	} else {
		sigc := commands.ForwardAllSignals(ctx, task)
		defer commands.StopCatch(sigc)
	}
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return ExitCodeError{
			exitCode: int(code),
		}
	}
	return nil
}

func createContainer(cmd *cobra.Command, ctx context.Context, client *containerd.Client, args []string, platform string, flagI, flagT, flagD bool) (containerd.Container, string, namestore.NameStore, error) {
	// simulate the behavior of double dash
	newArg := []string{}
	if len(args) >= 2 && args[1] == "--" {
		newArg = append(newArg, args[:1]...)
		newArg = append(newArg, args[2:]...)
		args = newArg
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, "", nil, err
	}

	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		id    = idgen.GenerateID()
	)

	cidfile, err := cmd.Flags().GetString("cidfile")
	if err != nil {
		return nil, "", nil, err
	}
	if cidfile != "" {
		if err := writeCIDFile(cidfile, id); err != nil {
			return nil, "", nil, err
		}
	}

	dataStore, err := getDataStore(cmd)
	if err != nil {
		return nil, "", nil, err
	}

	stateDir, err := getContainerStateDirPath(cmd, dataStore, id)
	if err != nil {
		return nil, "", nil, err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, "", nil, err
	}

	opts = append(opts,
		oci.WithDefaultSpec(),
	)

	opts, err = setPlatformOptions(opts, cmd, id)
	if err != nil {
		return nil, "", nil, err
	}

	rootfsOpts, rootfsCOpts, ensuredImage, err := generateRootfsOpts(ctx, client, platform, cmd, args, id)
	if err != nil {
		return nil, "", nil, err
	}
	opts = append(opts, rootfsOpts...)
	cOpts = append(cOpts, rootfsCOpts...)

	wd, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return nil, "", nil, err
	}
	if wd != "" {
		opts = append(opts, oci.WithProcessCwd(wd))
	}

	envFile, err := cmd.Flags().GetStringSlice("env-file")
	if err != nil {
		return nil, "", nil, err
	}
	if envFiles := strutil.DedupeStrSlice(envFile); len(envFiles) > 0 {
		env, err := parseEnvVars(envFiles)
		if err != nil {
			return nil, "", nil, err
		}
		opts = append(opts, oci.WithEnv(env))
	}

	env, err := cmd.Flags().GetStringArray("env")
	if err != nil {
		return nil, "", nil, err
	}
	if env := strutil.DedupeStrSlice(env); len(env) > 0 {
		opts = append(opts, oci.WithEnv(env))
	}

	if flagI {
		if flagD {
			return nil, "", nil, errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		if flagD {
			return nil, "", nil, errors.New("currently flag -t and -d cannot be specified together (FIXME)")
		}
		opts = append(opts, oci.WithTTY)
	}

	mountOpts, anonVolumes, mountPoints, err := generateMountOpts(cmd, ctx, client, ensuredImage)
	if err != nil {
		return nil, "", nil, err
	} else {
		opts = append(opts, mountOpts...)
	}

	var logURI string
	if flagD {
		// json-file is the built-in and default log driver for nerdctl
		logDriver, err := cmd.Flags().GetString("log-driver")
		if err != nil {
			return nil, "", nil, err
		}
		logOptMap, err := parseKVStringsMapFromLogOpt(cmd, logDriver)
		if err != nil {
			return nil, "", nil, err
		}
		logConfig := &logging.LogConfig{
			Driver: logDriver,
			Opts:   logOptMap,
		}
		logConfigB, err := json.Marshal(logConfig)
		if err != nil {
			return nil, "", nil, err
		}
		logConfigFilePath := logging.LogConfigFilePath(dataStore, ns, id)
		if err = os.WriteFile(logConfigFilePath, logConfigB, 0600); err != nil {
			return nil, "", nil, err
		}
		if lu, err := generateLogURI(dataStore); err != nil {
			return nil, "", nil, err
		} else if lu != nil {
			logURI = lu.String()
		}
	}

	restartValue, err := cmd.Flags().GetString("restart")
	if err != nil {
		return nil, "", nil, err
	}
	restartOpts, err := generateRestartOpts(ctx, client, restartValue, logURI)
	if err != nil {
		return nil, "", nil, err
	}
	cOpts = append(cOpts, restartOpts...)

	netOpts, netSlice, ipAddress, ports, err := generateNetOpts(cmd, dataStore, stateDir, ns, id)
	if err != nil {
		return nil, "", nil, err
	}
	opts = append(opts, netOpts...)

	hostname := id[0:12]
	customHostname, err := cmd.Flags().GetString("hostname")
	if err != nil {
		return nil, "", nil, err
	}
	if customHostname != "" {
		hostname = customHostname
	}
	opts = append(opts, oci.WithHostname(hostname))
	// `/etc/hostname` does not exist on FreeBSD
	if runtime.GOOS == "linux" {
		hostnamePath := filepath.Join(stateDir, "hostname")
		if err := os.WriteFile(hostnamePath, []byte(hostname+"\n"), 0644); err != nil {
			return nil, "", nil, err
		}
		opts = append(opts, withCustomEtcHostname(hostnamePath))
	}

	hookOpt, err := withNerdctlOCIHook(cmd, id, stateDir)
	if err != nil {
		return nil, "", nil, err
	}
	opts = append(opts, hookOpt)

	if uOpts, err := generateUserOpts(cmd); err != nil {
		return nil, "", nil, err
	} else {
		opts = append(opts, uOpts...)
	}

	rtCOpts, err := generateRuntimeCOpts(cmd)
	if err != nil {
		return nil, "", nil, err
	}
	cOpts = append(cOpts, rtCOpts...)

	lCOpts, err := withContainerLabels(cmd)
	if err != nil {
		return nil, "", nil, err
	}
	cOpts = append(cOpts, lCOpts...)

	var containerNameStore namestore.NameStore
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return nil, "", nil, err
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
		containerNameStore, err = namestore.New(dataStore, ns)
		if err != nil {
			return nil, "", nil, err
		}
		if err := containerNameStore.Acquire(name, id); err != nil {
			return nil, "", nil, err
		}
	}

	var pidFile string
	if cmd.Flags().Lookup("pidfile").Changed {
		pidFile, err = cmd.Flags().GetString("pidfile")
		if err != nil {
			return nil, "", nil, err
		}
	}

	extraHosts, err := cmd.Flags().GetStringSlice("add-host")
	if err != nil {
		return nil, "", nil, err
	}
	extraHosts = strutil.DedupeStrSlice(extraHosts)
	for _, host := range extraHosts {
		if _, err := dopts.ValidateExtraHost(host); err != nil {
			return nil, "", nil, err
		}
	}
	ilOpt, err := withInternalLabels(ns, name, hostname, stateDir, extraHosts, netSlice, ipAddress, ports, logURI, anonVolumes, pidFile, platform, mountPoints)
	if err != nil {
		return nil, "", nil, err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, propagateContainerdLabelsToOCIAnnotations())

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)
	cOpts = append(cOpts, spec)

	logrus.Debugf("final cOpts is %v", cOpts)
	container, err := client.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return nil, "", nil, err
	}
	return container, dataStore, containerNameStore, nil
}

func generateRootfsOpts(ctx context.Context, client *containerd.Client, platform string, cmd *cobra.Command, args []string, id string) ([]oci.SpecOpts, []containerd.NewContainerOpts, *imgutil.EnsuredImage, error) {
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
		ensured, err = ensureImage(cmd, ctx, client, rawRef, ocispecPlatforms, pull, nil, false)
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
		binaryNameInContainer := ""
		binaryPath, err := exec.LookPath(initBinary)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return nil, nil, nil, fmt.Errorf(`init binary %q not found`, initBinary)
			}
			return nil, nil, nil, err
		}
		binaryNameInContainer = filepath.Join("/sbin", filepath.Base(initBinary))
		opts = append(opts, func(_ context.Context, _ oci.Client, _ *containers.Container, spec *oci.Spec) error {
			spec.Process.Args = append([]string{binaryNameInContainer, "--"}, spec.Process.Args...)
			spec.Mounts = append([]specs.Mount{{
				Destination: binaryNameInContainer,
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
//
func withBindMountHostIPC(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		if path.Clean(m.Destination) == "/dev/shm" {
			newM := specs.Mount{
				Destination: "/dev/shm",
				Type:        "bind",
				Source:      "/dev/shm",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
		if path.Clean(m.Destination) == "/dev/mqueue" {
			newM := specs.Mount{
				Destination: "/dev/mqueue",
				Type:        "bind",
				Source:      "/dev/mqueue",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
	}
	return nil
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

func generateLogURI(dataStore string) (*url.URL, error) {
	selfExe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := map[string]string{
		logging.MagicArgv1: dataStore,
	}
	if runtime.GOOS == "windows" {
		return nil, nil
	}

	return cio.LogURIGenerator("binary", selfExe, args)
}

func withNerdctlOCIHook(cmd *cobra.Command, id, stateDir string) (oci.SpecOpts, error) {
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

func getContainerStateDirPath(cmd *cobra.Command, dataStore, id string) (string, error) {
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return "", err
	}
	if ns == "" {
		return "", errors.New("namespace is required")
	}
	if strings.Contains(ns, "/") {
		return "", errors.New("namespace with '/' is unsupported")
	}
	return filepath.Join(dataStore, "containers", ns, id), nil
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
	labels, err := opts.ReadKVStrings(labelsFilePath, labelsMap)
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
	return logOptMap, nil
}

func withInternalLabels(ns, name, hostname, containerStateDir string, extraHosts, networks []string, ipAddress string, ports []gocni.PortMapping, logURI string, anonVolumes []string, pidFile, platform string, mountPoints []*mountutil.Processed) (containerd.NewContainerOpts, error) {
	m := make(map[string]string)
	m[labels.Namespace] = ns
	if name != "" {
		m[labels.Name] = name
	}
	m[labels.Hostname] = hostname
	extraHostsJSON, err := json.Marshal(extraHosts)
	if err != nil {
		return nil, err
	}
	m[labels.ExtraHosts] = string(extraHostsJSON)
	m[labels.StateDir] = containerStateDir
	networksJSON, err := json.Marshal(networks)
	if err != nil {
		return nil, err
	}
	m[labels.Networks] = string(networksJSON)
	if len(ports) > 0 {
		portsJSON, err := json.Marshal(ports)
		if err != nil {
			return nil, err
		}
		m[labels.Ports] = string(portsJSON)
	}
	if logURI != "" {
		m[labels.LogURI] = logURI
	}
	if len(anonVolumes) > 0 {
		anonVolumeJSON, err := json.Marshal(anonVolumes)
		if err != nil {
			return nil, err
		}
		m[labels.AnonymousVolumes] = string(anonVolumeJSON)
	}

	if pidFile != "" {
		m[labels.PIDFile] = pidFile
	}

	if ipAddress != "" {
		m[labels.IPAddress] = ipAddress
	}

	m[labels.Platform], err = platformutil.NormalizeString(platform)
	if err != nil {
		return nil, err
	}

	if len(mountPoints) > 0 {
		mounts := dockercompatMounts(mountPoints)
		mountPointsJSON, err := json.Marshal(mounts)
		if err != nil {
			return nil, err
		}
		m[labels.Mounts] = string(mountPointsJSON)
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

func parseEnvVars(paths []string) ([]string, error) {
	vars := make([]string, 0)
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open env file %s: %w", path, err)
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			// skip comment lines
			if strings.HasPrefix(line, "#") {
				continue
			}
			vars = append(vars, line)
		}
		if err = sc.Err(); err != nil {
			return nil, err
		}
	}
	return vars, nil
}
