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
	"github.com/containerd/nerdctl/pkg/portutil"
	"github.com/containerd/nerdctl/pkg/resolvconf"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/docker/cli/opts"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var runCommand = &cli.Command{
	Name:         "run",
	Usage:        "Run a command in a new container",
	Action:       runAction,
	BashComplete: runBashComplete,
	HideHelp:     true, // built-in "-h" help conflicts with the short form of `--hostname`
	Description: func() string {
		var description string
		switch runtime.GOOS {
		case "windows":
			description += "WARNING: `nerdctl run` is experimental on Windows and currently broken (https://github.com/containerd/nerdctl/issues/28)"
		case "freebsd":
			description += "WARNING: `nerdctl run` is experimental on FreeBSD and currently requires `--net=none` (https://github.com/containerd/nerdctl/blob/master/docs/freebsd.md)"
		}
		return description
	}(),
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "help",
			// No "-h" alias for "--help", because "-h" for "--hostname".
			Usage: "show help",
		},
		&cli.BoolFlag{
			Name:    "tty",
			Aliases: []string{"t"},
			Usage:   "(Currently -t needs to correspond to -i)",
		},
		&cli.BoolFlag{
			Name:    "interactive",
			Aliases: []string{"i"},
			Usage:   "Keep STDIN open even if not attached",
		},
		&cli.BoolFlag{
			Name:    "detach",
			Aliases: []string{"d"},
			Usage:   "Run container in background and print container ID",
		},
		&cli.StringFlag{
			Name:  "restart",
			Usage: "Restart policy to apply when a container exits (implemented values: \"no\"|\"always\")",
			Value: "no",
		},
		&cli.BoolFlag{
			Name:  "rm",
			Usage: "Automatically remove the container when it exits",
		},
		&cli.StringFlag{
			Name:  "pull",
			Usage: "Pull image before running (\"always\"|\"missing\"|\"never\")",
			Value: "missing",
		},
		// network flags
		&cli.StringSliceFlag{
			Name:    "network",
			Aliases: []string{"net"},
			Usage:   "Connect a container to a network (\"bridge\"|\"host\"|\"none\")",
			Value:   cli.NewStringSlice(netutil.DefaultNetworkName),
		},
		&cli.StringSliceFlag{
			Name:  "dns",
			Usage: "Set custom DNS servers",
		},
		&cli.StringSliceFlag{
			Name:    "publish",
			Aliases: []string{"p"},
			Usage:   "Publish a container's port(s) to the host",
		},
		&cli.StringFlag{
			Name:    "hostname",
			Aliases: []string{"h"},
			Usage:   "Container host name",
		},
		// cgroup flags
		&cli.Float64Flag{
			Name:  "cpus",
			Usage: "Number of CPUs",
		},
		&cli.StringFlag{
			Name:    "memory",
			Aliases: []string{"m"},
			Usage:   "Memory limit",
		},
		// Enable host pid namespace
		&cli.StringFlag{
			Name:  "pid",
			Usage: "PID namespace to use",
		},
		&cli.IntFlag{
			Name:  "pids-limit",
			Usage: "Tune container pids limit (set -1 for unlimited)",
			Value: -1,
		},
		&cli.StringFlag{
			Name:  "cgroupns",
			Usage: "Cgroup namespace to use, the default depends on the cgroup version (\"host\"|\"private\")",
			Value: defaults.CgroupnsMode(),
		},
		&cli.StringFlag{
			Name:  "cpuset-cpus",
			Usage: "CPUs in which to allow execution (0-3, 0,1)",
		},
		&cli.IntFlag{
			Name:  "cpu-shares",
			Usage: "CPU shares (relative weight)",
		},
		&cli.StringSliceFlag{
			Name:  "device",
			Usage: "Add a host device to the container",
		},
		// user flags
		&cli.StringFlag{
			Name:    "user",
			Aliases: []string{"u"},
			Usage:   "Username or UID (format: <name|uid>[:<group|gid>])",
		},
		// security flags
		&cli.StringSliceFlag{
			Name:  "security-opt",
			Usage: "Security options",
		},
		&cli.StringSliceFlag{
			Name:  "cap-add",
			Usage: "Add Linux capabilities",
		},
		&cli.StringSliceFlag{
			Name:  "cap-drop",
			Usage: "Drop Linux capabilities",
		},
		&cli.BoolFlag{
			Name:  "privileged",
			Usage: "Give extended privileges to this container",
		},
		// runtime flags
		&cli.StringFlag{
			Name:  "runtime",
			Usage: "Runtime to use for this container, e.g. \"crun\", or \"io.containerd.runsc.v1\"",
			Value: defaults.Runtime,
		},
		&cli.StringSliceFlag{
			Name:  "sysctl",
			Usage: "Sysctl options",
		},
		&cli.StringSliceFlag{
			Name:  "gpus",
			Usage: "GPU devices to add to the container ('all' to pass all GPUs)",
		},
		// volume flags
		&cli.StringSliceFlag{
			Name:    "volume",
			Aliases: []string{"v"},
			Usage:   "Bind mount a volume",
		},
		// rootfs flags
		&cli.BoolFlag{
			Name:  "read-only",
			Usage: "Mount the container's root filesystem as read only",
		},
		// rootfs flags (from Podman)
		&cli.BoolFlag{
			Name:  "rootfs",
			Usage: "The first argument is not an image but the rootfs to the exploded container",
		},
		// env flags
		&cli.StringFlag{
			Name:  "entrypoint",
			Usage: "Overwrite the default ENTRYPOINT of the image",
		},
		&cli.StringFlag{
			Name:    "workdir",
			Aliases: []string{"w"},
			Usage:   "Working directory inside the container",
		},
		&cli.StringSliceFlag{
			Name:    "env",
			Aliases: []string{"e"},
			Usage:   "Set environment variables",
		},
		&cli.StringSliceFlag{
			Name:  "add-host",
			Usage: "Add a custom host-to-IP mapping (host:ip)",
		},
		&cli.StringSliceFlag{
			Name:  "env-file",
			Usage: "Set environment variables from file",
		},
		// metadata flags
		&cli.StringFlag{
			Name:  "name",
			Usage: "Assign a name to the container",
		},
		&cli.StringSliceFlag{
			Name:    "label",
			Aliases: []string{"l"},
			Usage:   "Set meta data on a container",
		},
		&cli.StringSliceFlag{
			Name:  "label-file",
			Usage: "Read in a line delimited file of labels",
		},
		&cli.StringFlag{
			Name:  "cidfile",
			Usage: "Write the container ID to the file",
		},
		// shared memory flags
		&cli.StringFlag{
			Name:  "shm-size",
			Usage: "Size of /dev/shm",
		},
		&cli.StringFlag{
			Name:  "pidfile",
			Usage: "file path to write the task's pid",
		},
		&cli.StringSliceFlag{
			Name:  "ulimit",
			Usage: "Ulimit options",
		},
	},
}

// runAction is heavily based on ctr implementation:
// https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
//
// FIXME: split to smaller functions
func runAction(clicontext *cli.Context) error {
	if clicontext.Bool("help") {
		return cli.ShowCommandHelp(clicontext, "run")
	}

	if clicontext.NArg() < 1 {
		return errors.New("image name needs to be specified")
	}

	ns := clicontext.String("namespace")

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		id    = idgen.GenerateID()
	)

	if cidfile := clicontext.String("cidfile"); cidfile != "" {
		if err := writeCIDFile(cidfile, id); err != nil {
			return err
		}
	}

	dataStore, err := getDataStore(clicontext)
	if err != nil {
		return err
	}

	stateDir, err := getContainerStateDirPath(clicontext, dataStore, id)
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

	rootfsOpts, rootfsCOpts, ensuredImage, err := generateRootfsOpts(ctx, client, clicontext, id)
	if err != nil {
		return err
	}
	opts = append(opts, rootfsOpts...)
	cOpts = append(cOpts, rootfsCOpts...)

	if wd := clicontext.String("workdir"); wd != "" {
		opts = append(opts, oci.WithProcessCwd(wd))
	}

	if envFiles := strutil.DedupeStrSlice(clicontext.StringSlice("env-file")); len(envFiles) > 0 {
		env, err := parseEnvVars(envFiles)
		if err != nil {
			return err
		}
		opts = append(opts, oci.WithEnv(env))
	}

	if env := strutil.DedupeStrSlice(clicontext.StringSlice("env")); len(env) > 0 {
		opts = append(opts, oci.WithEnv(env))
	}

	flagI := clicontext.Bool("i")
	flagT := clicontext.Bool("t")
	flagD := clicontext.Bool("d")

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

	mountOpts, anonVolumes, err := generateMountOpts(clicontext, ctx, client, ensuredImage)
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

	restartOpts, err := generateRestartOpts(clicontext.String("restart"), logURI)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, restartOpts...)

	// DedupeStrSlice is required as a workaround for urfave/cli bug
	// https://github.com/containerd/nerdctl/issues/108
	// https://github.com/urfave/cli/issues/1254
	portSlice := strutil.DedupeStrSlice(clicontext.StringSlice("p"))
	netSlice := strutil.DedupeStrSlice(clicontext.StringSlice("net"))

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
		e := &netutil.CNIEnv{
			Path:        clicontext.String("cni-path"),
			NetconfPath: clicontext.String("cni-netconfpath"),
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
				return errors.Errorf("no such network: %q", netstr)
			}
		}

		resolvConfPath := filepath.Join(stateDir, "resolv.conf")
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
		nameServers := strutil.DedupeStrSlice(clicontext.StringSlice("dns"))
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
		return errors.Errorf("unexpected network type %v", netType)
	}

	hostname := id[0:12]
	if customHostname := clicontext.String("hostname"); customHostname != "" {
		hostname = customHostname
	}
	opts = append(opts, oci.WithHostname(hostname))

	hookOpt, err := withNerdctlOCIHook(clicontext, id, stateDir)
	if err != nil {
		return err
	}
	opts = append(opts, hookOpt)

	if cgOpts, err := generateCgroupOpts(clicontext, id); err != nil {
		return err
	} else {
		opts = append(opts, cgOpts...)
	}

	if uOpts, err := generateUserOpts(clicontext); err != nil {
		return err
	} else {
		opts = append(opts, uOpts...)
	}

	securityOptsMaps := strutil.ConvertKVStringsToMap(strutil.DedupeStrSlice(clicontext.StringSlice("security-opt")))
	if secOpts, err := generateSecurityOpts(securityOptsMaps); err != nil {
		return err
	} else {
		opts = append(opts, secOpts...)
	}

	if capOpts, err := generateCapOpts(
		strutil.DedupeStrSlice(clicontext.StringSlice("cap-add")),
		strutil.DedupeStrSlice(clicontext.StringSlice("cap-drop"))); err != nil {
		return err
	} else {
		opts = append(opts, capOpts...)
	}

	if clicontext.Bool("privileged") {
		opts = append(opts, privilegedOpts...)
	}

	if shmSize := clicontext.String("shm-size"); len(shmSize) > 0 {
		shmBytes, err := units.RAMInBytes(shmSize)
		if err != nil {
			return err
		}
		opts = append(opts, oci.WithDevShmSize(shmBytes/1024))
	}

	pidNs := strings.ToLower(clicontext.String("pid"))
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

	ulimitOpts, err := generateUlimitsOpts(clicontext)
	if err != nil {
		return err
	}
	opts = append(opts, ulimitOpts...)

	rtCOpts, err := generateRuntimeCOpts(clicontext)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, rtCOpts...)

	lCOpts, err := withContainerLabels(clicontext)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, lCOpts...)

	var containerNameStore namestore.NameStore
	name := clicontext.String("name")
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
	if clicontext.IsSet("pidfile") {
		pidFile = clicontext.String("pidfile")
	}

	extraHosts := strutil.DedupeStrSlice(clicontext.StringSlice("add-host"))

	ilOpt, err := withInternalLabels(ns, name, hostname, stateDir, extraHosts, netSlice, ports, logURI, anonVolumes, pidFile)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, propagateContainerdLabelsToOCIAnnotations())

	opts = append(opts, WithSysctls(strutil.ConvertKVStringsToMap(clicontext.StringSlice("sysctl"))))

	gpuOpt, err := parseGPUOpts(clicontext.StringSlice("gpus"))
	if err != nil {
		return err
	}
	opts = append(opts, gpuOpt...)

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)
	cOpts = append(cOpts, spec)

	container, err := client.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return err
	}
	if clicontext.Bool("rm") {
		if flagD {
			return errors.New("flag -d and --rm cannot be specified together")
		}
		defer func() {
			const removeAnonVolumes = true
			if removeErr := removeContainer(clicontext, ctx, client, ns, id, id, true, dataStore, stateDir, containerNameStore, removeAnonVolumes); removeErr != nil {
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
			if clicontext.Bool("rm") {
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
		fmt.Fprintf(clicontext.App.Writer, "%s\n", id)
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
		return cli.NewExitError("", int(code))
	}
	return nil
}

func generateRootfsOpts(ctx context.Context, client *containerd.Client, clicontext *cli.Context, id string) ([]oci.SpecOpts, []containerd.NewContainerOpts, *imgutil.EnsuredImage, error) {
	imageless := clicontext.Bool("rootfs")
	var (
		ensured *imgutil.EnsuredImage
		err     error
	)
	if !imageless {
		ensured, err = imgutil.EnsureImage(ctx, client, clicontext.App.Writer, clicontext.String("snapshotter"), clicontext.Args().First(),
			clicontext.String("pull"), clicontext.Bool("insecure-registry"))
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
		absRootfs, err := filepath.Abs(clicontext.Args().First())
		if err != nil {
			return nil, nil, nil, err
		}
		opts = append(opts, oci.WithRootFSPath(absRootfs), oci.WithDefaultPathEnv)
	}

	// NOTE: "--entrypoint" can be set to an empty string, see TestRunEntrypoint* in run_test.go .
	if !imageless && !clicontext.IsSet("entrypoint") {
		opts = append(opts, oci.WithImageConfigArgs(ensured.Image, clicontext.Args().Tail()))
	} else {
		if !imageless {
			opts = append(opts, oci.WithImageConfig(ensured.Image))
		}
		var processArgs []string
		if entrypoint := clicontext.String("entrypoint"); entrypoint != "" {
			processArgs = append(processArgs, entrypoint)
		}
		if clicontext.NArg() > 1 {
			processArgs = append(processArgs, clicontext.Args().Tail()...)
		}
		if len(processArgs) == 0 {
			// error message is from Podman
			return nil, nil, nil, errors.New("no command or entrypoint provided, and no CMD or ENTRYPOINT from image")
		}
		opts = append(opts, oci.WithProcessArgs(processArgs...))
	}

	if clicontext.Bool("read-only") {
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

func withNerdctlOCIHook(clicontext *cli.Context, id, stateDir string) (oci.SpecOpts, error) {
	selfExe, f := globalFlags(clicontext)
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
		return nil, errors.Errorf("unsupported restart type %q, supported types are: \"no\",  \"always\"", restartFlag)
	}
}

func getContainerStateDirPath(clicontext *cli.Context, dataStore, id string) (string, error) {
	ns := clicontext.String("namespace")
	if ns == "" {
		return "", errors.New("namespace is required")
	}
	if strings.Contains(ns, "/") {
		return "", errors.New("namespace with '/' is unsupported")
	}
	return filepath.Join(dataStore, "containers", ns, id), nil
}

func withContainerLabels(clicontext *cli.Context) ([]containerd.NewContainerOpts, error) {
	labelsMap := strutil.DedupeStrSlice(clicontext.StringSlice("label"))
	labelsFilePath := strutil.DedupeStrSlice(clicontext.StringSlice("label-file"))
	labels, err := opts.ReadKVStrings(labelsFilePath, labelsMap)
	if err != nil {
		return nil, err
	}
	o := containerd.WithAdditionalContainerLabels(strutil.ConvertKVStringsToMap(labels))
	return []containerd.NewContainerOpts{o}, nil
}

func withInternalLabels(ns, name, hostname, containerStateDir string, extraHosts, networks []string, ports []gocni.PortMapping, logURI string, anonVolumes []string, pidFile string) (containerd.NewContainerOpts, error) {
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

	return containerd.WithAdditionalContainerLabels(m), nil
}

func propagateContainerdLabelsToOCIAnnotations() oci.SpecOpts {
	return func(ctx context.Context, oc oci.Client, c *containers.Container, s *oci.Spec) error {
		return oci.WithAnnotations(c.Labels)(ctx, oc, c, s)
	}
}

func writeCIDFile(path, id string) error {
	if _, err := os.Stat(path); err == nil {
		return errors.Errorf("container ID file found, make sure the other container isn't running or delete %s", path)
	} else if errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(path)
		defer f.Close()
		if err != nil {
			return errors.Errorf("failed to create the container ID file: %s", err)
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
			return nil, errors.Wrapf(err, "failed to open env file %s", path)
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
