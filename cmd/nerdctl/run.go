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
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/runtime/restart"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/dnsutil"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/netutil/nettype"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/portutil"
	"github.com/containerd/nerdctl/pkg/resolvconf"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/docker/cli/opts"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	shortHelp := "Run a command in a new container"
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
		Use:               "run IMAGE [COMMAND] [ARG...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              runAction,
		ValidArgsFunction: runShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	runCommand.Flags().SetInterspersed(false)

	// No "-h" alias for "--help", because "-h" for "--hostname".
	runCommand.Flags().Bool("help", false, "show help")

	runCommand.Flags().BoolP("tty", "t", false, "(Currently -t needs to correspond to -i)")
	runCommand.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")
	runCommand.Flags().BoolP("detach", "d", false, "Run container in background and print container ID")
	runCommand.Flags().String("restart", "no", `Restart policy to apply when a container exits (implemented values: "no"|"always")`)
	runCommand.RegisterFlagCompletionFunc("restart", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"no", "always"}, cobra.ShellCompDirectiveNoFileComp
	})
	runCommand.Flags().Bool("rm", false, "Automatically remove the container when it exits")
	runCommand.Flags().String("pull", "missing", `Pull image before running ("always"|"missing"|"never")`)
	runCommand.RegisterFlagCompletionFunc("pull", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"always", "missing", "never"}, cobra.ShellCompDirectiveNoFileComp
	})

	// #region platform flags
	runCommand.Flags().String("platform", "", "Set platform (e.g. \"amd64\", \"arm64\")") // not a slice, and there is no --all-platforms
	runCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	// #endregion

	// #region network flags
	runCommand.Flags().StringSlice("network", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none")`)
	runCommand.RegisterFlagCompletionFunc("network", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return shellCompleteNetworkNames(cmd, []string{})
	})
	runCommand.Flags().StringSlice("net", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none")`)
	runCommand.RegisterFlagCompletionFunc("net", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return shellCompleteNetworkNames(cmd, []string{})
	})
	runCommand.Flags().StringSlice("dns", nil, "Set custom DNS servers")
	runCommand.Flags().StringSliceP("publish", "p", nil, "Publish a container's port(s) to the host")
	runCommand.Flags().StringP("hostname", "h", "", "Container host name")
	// #endregion

	// #region cgroups, namespaces, and ulimits flags
	runCommand.Flags().Float64("cpus", 0.0, "Number of CPUs")
	runCommand.Flags().StringP("memory", "m", "", "Memory limit")
	runCommand.Flags().String("pid", "", "PID namespace to use")
	runCommand.RegisterFlagCompletionFunc("pid", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host"}, cobra.ShellCompDirectiveNoFileComp
	})
	runCommand.Flags().Int("pids-limit", -1, "Tune container pids limit (set -1 for unlimited)")
	runCommand.Flags().String("cgroupns", defaults.CgroupnsMode(), `Cgroup namespace to use, the default depends on the cgroup version ("host"|"private")`)
	runCommand.RegisterFlagCompletionFunc("cgroupns", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host", "private"}, cobra.ShellCompDirectiveNoFileComp
	})
	runCommand.Flags().String("cpuset-cpus", "", "CPUs in which to allow execution (0-3, 0,1)")
	runCommand.Flags().Int("cpu-shares", 0, "CPU shares (relative weight)")
	runCommand.Flags().StringSlice("device", nil, "Add a host device to the container")
	runCommand.Flags().StringSlice("ulimit", nil, "Ulimit options")
	// #endregion

	// user flags
	runCommand.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")

	// #region security flags
	runCommand.Flags().StringSlice("security-opt", []string{}, "Security options")
	runCommand.RegisterFlagCompletionFunc("security-opt", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"seccomp=", "seccomp=unconfined", "apparmor=", "apparmor=" + defaults.AppArmorProfileName, "apparmor=unconfined", "no-new-privileges"}, cobra.ShellCompDirectiveNoFileComp
	})
	runCommand.Flags().StringSlice("cap-add", []string{}, "Add Linux capabilities")
	runCommand.RegisterFlagCompletionFunc("cap-add", capShellComplete)
	runCommand.Flags().StringSlice("cap-drop", []string{}, "Drop Linux capabilities")
	runCommand.RegisterFlagCompletionFunc("cap-drop", capShellComplete)
	runCommand.Flags().Bool("privileged", false, "Give extended privileges to this container")
	// #endregion

	// #region runtime flags
	runCommand.Flags().String("runtime", defaults.Runtime, "Runtime to use for this container, e.g. \"crun\", or \"io.containerd.runsc.v1\"")
	runCommand.Flags().StringSlice("sysctl", nil, "Sysctl options")
	runCommand.Flags().StringSlice("gpus", nil, "GPU devices to add to the container ('all' to pass all GPUs)")
	runCommand.RegisterFlagCompletionFunc("gpus", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"all"}, cobra.ShellCompDirectiveNoFileComp
	})
	// #endregion

	// #region mount flags
	runCommand.Flags().StringSliceP("volume", "v", nil, "Bind mount a volume")
	// #endregion

	// rootfs flags
	runCommand.Flags().Bool("read-only", false, "Mount the container's root filesystem as read only")
	// rootfs flags (from Podman)
	runCommand.Flags().Bool("rootfs", false, "The first agument is not an image but the rootfs to the exploded container")

	// #region env flags
	runCommand.Flags().String("entrypoint", "", "Overwrite the default ENTRYPOINT of the image")
	runCommand.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	runCommand.Flags().StringSliceP("env", "e", nil, "Set environment variables")
	runCommand.Flags().StringSlice("add-host", nil, "Add a custom host-to-IP mapping (host:ip)")
	runCommand.Flags().StringSlice("env-file", nil, "Set environment variables from file")

	// #region metadata flags
	runCommand.Flags().String("name", "", "Assign a name to the container")
	runCommand.Flags().StringSliceP("label", "l", nil, "Set metadata on container")
	runCommand.Flags().StringSlice("label-file", nil, "Set metadata on container from file")
	runCommand.Flags().String("cidfile", "", "Write the container ID to the file")
	runCommand.Flags().String("pidfile", "", "file path to write the task's pid")
	// #endregion

	// shared memory flags
	runCommand.Flags().String("shm-size", "", "Size of /dev/shm")

	return runCommand
}

// runAction is heavily based on ctr implementation:
// https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
//
// FIXME: split to smaller functions
func runAction(cmd *cobra.Command, args []string) error {
	// simulate the behavior of double dash
	newArg := []string{}
	if len(args) >= 2 && args[1] == "--" {
		newArg = append(newArg, args[:1]...)
		newArg = append(newArg, args[2:]...)
		args = newArg
	}

	if len(args) < 1 {
		return errors.New("image name needs to be specified")
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	var clientOpts []containerd.ClientOpt
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return err
	}
	if platform != "" {
		if canExec, canExecErr := platformutil.CanExecProbably(platform); !canExec {
			warn := fmt.Sprintf("Platform %q seems incompatible with the host platform %q. If you see \"exec format error\", see https://github.com/containerd/nerdctl/blob/master/docs/multi-platform.md",
				platform, platforms.DefaultString())
			if canExecErr != nil {
				logrus.WithError(canExecErr).Warn(warn)
			} else {
				logrus.Warn(warn)
			}
		}
		platformParsed, err := platforms.Parse(platform)
		if err != nil {
			return err
		}
		platformM := platforms.Only(platformParsed)
		clientOpts = append(clientOpts, containerd.WithDefaultPlatform(platformM))
	}
	client, ctx, cancel, err := newClient(cmd, clientOpts...)
	if err != nil {
		return err
	}
	defer cancel()

	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		id    = idgen.GenerateID()
	)

	cidfile, err := cmd.Flags().GetString("cidfile")
	if err != nil {
		return err
	}
	if cidfile != "" {
		if err := writeCIDFile(cidfile, id); err != nil {
			return err
		}
	}

	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}

	stateDir, err := getContainerStateDirPath(cmd, dataStore, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}

	opts = append(opts,
		oci.WithDefaultSpec(),
		oci.WithDefaultUnixDevices,
		WithoutRunMount(), // unmount default tmpfs on "/run": https://github.com/containerd/nerdctl/issues/157
	)

	if runtime.GOOS == "linux" {
		opts = append(opts,
			oci.WithMounts([]specs.Mount{
				{Type: "cgroup", Source: "cgroup", Destination: "/sys/fs/cgroup", Options: []string{"ro", "nosuid", "noexec", "nodev"}},
			}))
	}

	rootfsOpts, rootfsCOpts, ensuredImage, err := generateRootfsOpts(ctx, client, platform, cmd, args, id)
	if err != nil {
		return err
	}
	opts = append(opts, rootfsOpts...)
	cOpts = append(cOpts, rootfsCOpts...)

	wd, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return err
	}
	if wd != "" {
		opts = append(opts, oci.WithProcessCwd(wd))
	}

	envFile, err := cmd.Flags().GetStringSlice("env-file")
	if err != nil {
		return err
	}
	if envFiles := strutil.DedupeStrSlice(envFile); len(envFiles) > 0 {
		env, err := parseEnvVars(envFiles)
		if err != nil {
			return err
		}
		opts = append(opts, oci.WithEnv(env))
	}

	env, err := cmd.Flags().GetStringSlice("env")
	if err != nil {
		return err
	}
	if env := strutil.DedupeStrSlice(env); len(env) > 0 {
		opts = append(opts, oci.WithEnv(env))
	}

	flagI, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}
	flagT, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return err
	}
	flagD, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}

	if flagI {
		if flagD {
			return errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		if flagD {
			return errors.New("currently flag -t and -d cannot be specified together (FIXME)")
		}
		if !flagI {
			return errors.New("currently flag -t needs -i to be specified together (FIXME)")
		}
		opts = append(opts, oci.WithTTY)
	}

	mountOpts, anonVolumes, err := generateMountOpts(cmd, ctx, client, ensuredImage)
	if err != nil {
		return err
	} else {
		opts = append(opts, mountOpts...)
	}

	var logURI string
	if flagD {
		if lu, err := generateLogURI(dataStore); err != nil {
			return err
		} else if lu != nil {
			logURI = lu.String()
		}
	}

	restartValue, err := cmd.Flags().GetString("restart")
	if err != nil {
		return err
	}
	restartOpts, err := generateRestartOpts(restartValue, logURI)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, restartOpts...)

	// DedupeStrSlice is required as a workaround for urfave/cli bug
	// https://github.com/containerd/nerdctl/issues/108
	// https://github.com/urfave/cli/issues/1254
	portSlice, err := cmd.Flags().GetStringSlice("publish")
	if err != nil {
		return err
	}
	netSlice, err := getNetworkSlice(cmd)
	if err != nil {
		return err
	}

	ports := make([]gocni.PortMapping, 0)
	netType, err := nettype.Detect(netSlice)
	if err != nil {
		return err
	}

	switch netType {
	case nettype.None:
		// NOP
	case nettype.Host:
		opts = append(opts, oci.WithHostNamespace(specs.NetworkNamespace), oci.WithHostHostsFile, oci.WithHostResolvconf)
	case nettype.CNI:
		// We only verify flags and generate resolv.conf here.
		// The actual network is configured in the oci hook.
		cniPath, err := cmd.Flags().GetString("cni-path")
		if err != nil {
			return err
		}
		cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
		if err != nil {
			return err
		}
		e := &netutil.CNIEnv{
			Path:        cniPath,
			NetconfPath: cniNetconfpath,
		}
		ll, err := netutil.ConfigLists(e)
		if err != nil {
			return err
		}
		for _, netstr := range netSlice {
			var netconflist *netutil.NetworkConfigList
			for _, f := range ll {
				if f.Name == netstr {
					netconflist = f
					break
				}
			}
			if netconflist == nil {
				return fmt.Errorf("no such network: %q", netstr)
			}
		}

		resolvConfPath := filepath.Join(stateDir, "resolv.conf")
		dnsValue, err := cmd.Flags().GetStringSlice("dns")
		if err != nil {
			return err
		}
		conf, err := resolvconf.Get()
		if err != nil {
			return err
		}
		slirp4Dns := []string{}
		if rootlessutil.IsRootlessChild() {
			slirp4Dns, err = dnsutil.GetSlirp4netnsDns()
			if err != nil {
				return err
			}
		}
		conf, err = resolvconf.FilterResolvDNS(conf.Content, true)
		if err != nil {
			return err
		}
		searchDomains := resolvconf.GetSearchDomains(conf.Content)
		dnsOptions := resolvconf.GetOptions(conf.Content)
		nameServers := strutil.DedupeStrSlice(dnsValue)
		if len(nameServers) == 0 {
			nameServers = resolvconf.GetNameservers(conf.Content, resolvconf.IPv4)
		}
		if _, err := resolvconf.Build(resolvConfPath, append(slirp4Dns, nameServers...), searchDomains, dnsOptions); err != nil {
			return err
		}
		// the content of /etc/hosts is created in OCI Hook
		etcHostsPath, err := hostsstore.AllocHostsFile(dataStore, ns, id)
		if err != nil {
			return err
		}
		opts = append(opts, withCustomResolvConf(resolvConfPath), withCustomHosts(etcHostsPath))
		for _, p := range portSlice {
			pm, err := portutil.ParseFlagP(p)
			if err != nil {
				return err
			}
			ports = append(ports, pm...)
		}
	default:
		return fmt.Errorf("unexpected network type %v", netType)
	}

	hostname := id[0:12]
	customHostname, err := cmd.Flags().GetString("hostname")
	if err != nil {
		return err
	}
	if customHostname != "" {
		hostname = customHostname
	}
	opts = append(opts, oci.WithHostname(hostname))
	// `/etc/hostname` does not exist on FreeBSD
	if runtime.GOOS == "linux" {
		hostnamePath := filepath.Join(stateDir, "hostname")
		if err := os.WriteFile(hostnamePath, []byte(hostname+"\n"), 0644); err != nil {
			return err
		}
		opts = append(opts, withCustomEtcHostname(hostnamePath))
	}

	hookOpt, err := withNerdctlOCIHook(cmd, id, stateDir)
	if err != nil {
		return err
	}
	opts = append(opts, hookOpt)

	if cgOpts, err := generateCgroupOpts(cmd, id); err != nil {
		return err
	} else {
		opts = append(opts, cgOpts...)
	}

	if uOpts, err := generateUserOpts(cmd); err != nil {
		return err
	} else {
		opts = append(opts, uOpts...)
	}

	securityOpt, err := cmd.Flags().GetStringSlice("security-opt")
	if err != nil {
		return err
	}
	securityOptsMaps := strutil.ConvertKVStringsToMap(strutil.DedupeStrSlice(securityOpt))
	if secOpts, err := generateSecurityOpts(securityOptsMaps); err != nil {
		return err
	} else {
		opts = append(opts, secOpts...)
	}

	capAdd, err := cmd.Flags().GetStringSlice("cap-add")
	if err != nil {
		return err
	}
	capDrop, err := cmd.Flags().GetStringSlice("cap-drop")
	if err != nil {
		return err
	}
	if capOpts, err := generateCapOpts(
		strutil.DedupeStrSlice(capAdd),
		strutil.DedupeStrSlice(capDrop)); err != nil {
		return err
	} else {
		opts = append(opts, capOpts...)
	}

	privileged, err := cmd.Flags().GetBool("privileged")
	if err != nil {
		return err
	}
	if privileged {
		opts = append(opts, privilegedOpts...)
	}

	shmSize, err := cmd.Flags().GetString("shm-size")
	if err != nil {
		return err
	}
	if len(shmSize) > 0 {
		shmBytes, err := units.RAMInBytes(shmSize)
		if err != nil {
			return err
		}
		opts = append(opts, oci.WithDevShmSize(shmBytes/1024))
	}

	pidNs, err := cmd.Flags().GetString("pid")
	if err != nil {
		return err
	}
	pidNs = strings.ToLower(pidNs)
	if pidNs != "" {
		if pidNs != "host" {
			return fmt.Errorf("Invalid pid namespace. Set --pid=host to enable host pid namespace.")
		} else {
			opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
			if rootlessutil.IsRootless() {
				opts = append(opts, withBindMountHostProcfs)
			}
		}
	}

	ulimitOpts, err := generateUlimitsOpts(cmd)
	if err != nil {
		return err
	}
	opts = append(opts, ulimitOpts...)

	rtCOpts, err := generateRuntimeCOpts(cmd)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, rtCOpts...)

	lCOpts, err := withContainerLabels(cmd)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, lCOpts...)

	var containerNameStore namestore.NameStore
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}
	if name != "" {
		containerNameStore, err = namestore.New(dataStore, ns)
		if err != nil {
			return err
		}
		if err := containerNameStore.Acquire(name, id); err != nil {
			return err
		}
	}

	var pidFile string
	if cmd.Flags().Lookup("pidfile").Changed {
		pidFile, err = cmd.Flags().GetString("pidfile")
		if err != nil {
			return err
		}
	}

	extraHosts, err := cmd.Flags().GetStringSlice("add-host")
	if err != nil {
		return err
	}
	extraHosts = strutil.DedupeStrSlice(extraHosts)
	ilOpt, err := withInternalLabels(ns, name, hostname, stateDir, extraHosts, netSlice, ports, logURI, anonVolumes, pidFile, platform)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, propagateContainerdLabelsToOCIAnnotations())

	sysctl, err := cmd.Flags().GetStringSlice("sysctl")
	if err != nil {
		return err
	}
	opts = append(opts, WithSysctls(strutil.ConvertKVStringsToMap(sysctl)))

	gpus, err := cmd.Flags().GetStringSlice("gpus")
	if err != nil {
		return err
	}
	gpuOpt, err := parseGPUOpts(gpus)
	if err != nil {
		return err
	}
	opts = append(opts, gpuOpt...)

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)
	cOpts = append(cOpts, spec)

	logrus.Debugf("final cOpts is %v", cOpts)
	container, err := client.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return err
	}
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
			if removeErr := removeContainer(cmd, ctx, client, ns, id, id, true, dataStore, stateDir, containerNameStore, removeAnonVolumes); removeErr != nil {
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

func getNetworkSlice(cmd *cobra.Command) ([]string, error) {
	var netSlice = []string{}
	var networkSet = false
	if cmd.Flags().Lookup("network").Changed {
		network, err := cmd.Flags().GetStringSlice("network")
		if err != nil {
			return nil, err
		}
		netSlice = append(netSlice, network...)
		networkSet = true
	}
	if cmd.Flags().Lookup("net").Changed {
		net, err := cmd.Flags().GetStringSlice("net")
		if err != nil {
			return nil, err
		}
		netSlice = append(netSlice, net...)
		networkSet = true
	}

	if !networkSet {
		network, err := cmd.Flags().GetStringSlice("network")
		if err != nil {
			return nil, err
		}
		netSlice = append(netSlice, network...)
	}
	return netSlice, nil
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
		snapshotter, err := cmd.Flags().GetString("snapshotter")
		if err != nil {
			return nil, nil, nil, err
		}
		pull, err := cmd.Flags().GetString("pull")
		if err != nil {
			return nil, nil, nil, err
		}
		insecureRegistry, err := cmd.Flags().GetBool("insecure-registry")
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
		ensured, err = imgutil.EnsureImage(ctx, client, os.Stdout, snapshotter, args[0],
			pull, insecureRegistry, ocispecPlatforms)
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
	entrypoint, err := cmd.Flags().GetString("entrypoint")
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
		if entrypoint != "" {
			processArgs = append(processArgs, entrypoint)
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

	readonly, err := cmd.Flags().GetBool("read-only")
	if err != nil {
		return nil, nil, nil, err
	}
	if readonly {
		opts = append(opts, oci.WithRootFSReadonly())
	}
	return opts, cOpts, ensured, nil
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

func withCustomResolvConf(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind", mountutil.DefaultPropagationMode}, // writable
		})
		return nil
	}
}

func withCustomEtcHostname(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind", mountutil.DefaultPropagationMode}, // writable
		})
		return nil
	}
}

func withCustomHosts(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind", mountutil.DefaultPropagationMode}, // writable
		})
		return nil
	}
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

func generateRestartOpts(restartFlag, logURI string) ([]containerd.NewContainerOpts, error) {
	switch restartFlag {
	case "", "no":
		return nil, nil
	case "always":
		opts := []containerd.NewContainerOpts{restart.WithStatus(containerd.Running)}
		if logURI != "" {
			opts = append(opts, restart.WithLogURIString(logURI))
		}
		return opts, nil
	default:
		return nil, fmt.Errorf("unsupported restart type %q, supported types are: \"no\",  \"always\"", restartFlag)
	}
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
	labelsMap, err := cmd.Flags().GetStringSlice("label")
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
	o := containerd.WithAdditionalContainerLabels(strutil.ConvertKVStringsToMap(labels))
	return []containerd.NewContainerOpts{o}, nil
}

func withInternalLabels(ns, name, hostname, containerStateDir string, extraHosts, networks []string, ports []gocni.PortMapping, logURI string, anonVolumes []string, pidFile, platform string) (containerd.NewContainerOpts, error) {
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

	m[labels.Platform], err = platformutil.NormalizeString(platform)
	if err != nil {
		return nil, err
	}
	return containerd.WithAdditionalContainerLabels(m), nil
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
