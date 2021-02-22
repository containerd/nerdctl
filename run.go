/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AkihiroSuda/nerdctl/pkg/defaults"
	"github.com/AkihiroSuda/nerdctl/pkg/dnsutil"
	"github.com/AkihiroSuda/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil"
	"github.com/AkihiroSuda/nerdctl/pkg/labels"
	"github.com/AkihiroSuda/nerdctl/pkg/logging"
	"github.com/AkihiroSuda/nerdctl/pkg/namestore"
	"github.com/AkihiroSuda/nerdctl/pkg/netutil"
	"github.com/AkihiroSuda/nerdctl/pkg/portutil"
	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/runtime/restart"
	gocni "github.com/containerd/go-cni"
	"github.com/docker/cli/opts"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var runCommand = &cli.Command{
	Name:     "run",
	Usage:    "Run a command in a new container",
	Action:   runAction,
	HideHelp: true, // built-in "-h" help conflicts with the short form of `--hostname`
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
			Value: cli.NewStringSlice("8.8.8.8", "1.1.1.1"),
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
			Value: plugin.RuntimeRuncV2,
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
			Name:    "workdir",
			Aliases: []string{"w"},
			Usage:   "Working directory inside the container",
		},
		&cli.StringSliceFlag{
			Name:    "env",
			Aliases: []string{"e"},
			Usage:   "Set environment variables",
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
	},
}

// runAction is heavily based on ctr implementation:
// https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
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

	imageless := clicontext.Bool("rootfs")
	var ensured *imgutil.EnsuredImage
	if !imageless {
		ensured, err = imgutil.EnsureImage(ctx, client, clicontext.App.Writer, clicontext.String("snapshotter"), clicontext.Args().First(), clicontext.String("pull"))
		if err != nil {
			return err
		}
	}
	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		id    = genID()
	)

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
		oci.WithMounts([]specs.Mount{
			{Type: "cgroup", Source: "cgroup", Destination: "/sys/fs/cgroup", Options: []string{"ro"}},
		}),
	)

	if imageless {
		absRootfs, err := filepath.Abs(clicontext.Args().First())
		if err != nil {
			return err
		}
		opts = append(opts, oci.WithRootFSPath(absRootfs))
	} else {
		opts = append(opts, oci.WithImageConfig(ensured.Image))
		cOpts = append(cOpts,
			containerd.WithImage(ensured.Image),
			containerd.WithSnapshotter(ensured.Snapshotter),
			containerd.WithNewSnapshot(id, ensured.Image),
			containerd.WithImageStopSignal(ensured.Image, "SIGTERM"),
		)
	}

	if clicontext.Bool("read-only") {
		opts = append(opts, oci.WithRootFSReadonly())
	}

	if clicontext.NArg() > 1 {
		opts = append(opts, oci.WithProcessArgs(clicontext.Args().Tail()...))
	} else if imageless {
		// error message is from Podman
		return errors.New("no command or entrypoint provided, and no CMD or ENTRYPOINT from image")
	}

	if wd := clicontext.String("workdir"); wd != "" {
		opts = append(opts, oci.WithProcessCwd(wd))
	}

	if env := clicontext.StringSlice("env"); len(env) > 0 {
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

	if mountOpts, err := generateMountOpts(clicontext); err != nil {
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

	ports := make([]gocni.PortMapping, len(clicontext.StringSlice("p")))
	if len(clicontext.StringSlice("network")) != 1 {
		return errors.New("currently, number of networks must be 1")
	}
	switch netstr := clicontext.StringSlice("network")[0]; netstr {
	case "none":
		// NOP
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.NetworkNamespace), oci.WithHostHostsFile, oci.WithHostResolvconf)
	default:
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

		resolvConfPath := filepath.Join(stateDir, "resolv.conf")
		if err := dnsutil.WriteResolvConfFile(resolvConfPath, clicontext.StringSlice("dns")); err != nil {
			return err
		}
		// the content of /etc/hosts is created in OCI Hook
		etcHostsPath, err := hostsstore.AllocHostsFile(dataStore, ns, id)
		if err != nil {
			return err
		}
		opts = append(opts, withCustomResolvConf(resolvConfPath), withCustomHosts(etcHostsPath))
		for i, p := range clicontext.StringSlice("p") {
			pm, err := portutil.ParseFlagP(p)
			if err != nil {
				return err
			}
			ports[i] = *pm
		}
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

	securityOptsMaps := ConvertKVStringsToMap(clicontext.StringSlice("security-opt"))
	if secOpts, err := generateSecurityOpts(securityOptsMaps); err != nil {
		return err
	} else {
		opts = append(opts, secOpts...)
	}

	if capOpts, err := generateCapOpts(clicontext.StringSlice("cap-add"), clicontext.StringSlice("cap-drop")); err != nil {
		return err
	} else {
		opts = append(opts, capOpts...)
	}

	if clicontext.Bool("privileged") {
		opts = append(opts, privilegedOpts...)
	}

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
	ilOpt, err := withInternalLabels(ns, name, hostname, stateDir, clicontext.StringSlice("network"), ports)
	if err != nil {
		return err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, propagateContainerdLabelsToOCIAnnotations())

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
			if removeErr := removeContainer(clicontext, ctx, client, ns, id, id, true, dataStore, stateDir, containerNameStore); removeErr != nil {
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

	task, err := newTask(ctx, client, container, flagI, flagT, flagD, con, logURI)
	if err != nil {
		return err
	}
	var statusC <-chan containerd.ExitStatus
	if !flagD {
		defer func() {
			task.Delete(ctx)
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
	if _, err := task.Delete(ctx); err != nil {
		return err
	}
	if code != 0 {
		return cli.NewExitError("", int(code))
	}
	return nil
}

func genID() string {
	h := sha256.New()
	if err := binary.Write(h, binary.LittleEndian, time.Now().UnixNano()); err != nil {
		panic(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func withCustomResolvConf(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind"}, // writable
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
			Options:     []string{"bind"}, // writable
		})
		return nil
	}
}

func generateLogURI(dataStore string) (*url.URL, error) {
	selfExe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return nil, err
	}
	args := map[string]string{
		logging.MagicArgv1: dataStore,
	}
	return cio.LogURIGenerator("binary", selfExe, args)
}

func withNerdctlOCIHook(clicontext *cli.Context, id, stateDir string) (oci.SpecOpts, error) {
	selfExe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return nil, err
	}
	args := []string{
		os.Args[0],
		// FIXME: How to propagate all global flags?
		"--data-root=" + clicontext.String("data-root"),
		"--address=" + clicontext.String("address"),
		"--cni-path=" + clicontext.String("cni-path"),
		"--cni-netconfpath=" + clicontext.String("cni-netconfpath"),
		"internal",
		"oci-hook",
	}
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
	labelsMap := clicontext.StringSlice("label")
	labelsFilePath := clicontext.StringSlice("label-file")
	labels, err := opts.ReadKVStrings(labelsFilePath, labelsMap)
	if err != nil {
		return nil, err
	}
	o := containerd.WithAdditionalContainerLabels(ConvertKVStringsToMap(labels))
	return []containerd.NewContainerOpts{o}, nil
}

func withInternalLabels(ns, name, hostname, containerStateDir string, networks []string, ports []gocni.PortMapping) (containerd.NewContainerOpts, error) {
	m := make(map[string]string)
	m[labels.Namespace] = ns
	if name != "" {
		m[labels.Name] = name
	}
	m[labels.Hostname] = hostname
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
	return containerd.WithAdditionalContainerLabels(m), nil
}

func propagateContainerdLabelsToOCIAnnotations() oci.SpecOpts {
	return func(ctx context.Context, oc oci.Client, c *containers.Container, s *oci.Spec) error {
		return oci.WithAnnotations(c.Labels)(ctx, oc, c, s)
	}
}
