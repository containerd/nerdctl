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
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AkihiroSuda/nerdctl/pkg/imgutil"
	"github.com/AkihiroSuda/nerdctl/pkg/mountutil"
	"github.com/AkihiroSuda/nerdctl/pkg/portutil"
	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/runtime/restart"
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
			Name:    "tty",
			Aliases: []string{"t"},
			Usage:   "(Currently -t needs to correspond to -i)",
		},
		&cli.BoolFlag{
			Name:    "interactive",
			Aliases: []string{"i"},
			Usage:   "(Currently -i needs to correspond to -t)",
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
		&cli.StringFlag{
			Name:    "network",
			Aliases: []string{"net"},
			Usage:   "Connect a container to a network (\"bridge\"|\"host\"|\"none\")",
			Value:   "bridge",
		},
		&cli.StringSliceFlag{
			Name:  "dns",
			Usage: "Set custom DNS servers (only meaningful for \"bridge\" network)",
			Value: cli.NewStringSlice("8.8.8.8", "1.1.1.1"),
		},
		&cli.StringSliceFlag{
			Name:    "publish",
			Aliases: []string{"p"},
			Usage:   "Publish a container's port(s) to the host (Currently TCP only)",
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
			Value: defaultCgroupnsMode(),
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
		&cli.BoolFlag{
			Name:  "privileged",
			Usage: "Give extended privileges to this container",
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
		// misc flags
		&cli.StringFlag{
			Name:    "workdir",
			Aliases: []string{"w"},
			Usage:   "Working directory inside the container",
		},
	},
}

// runAction is heavily based on ctr implementation:
// https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
func runAction(clicontext *cli.Context) error {
	if clicontext.NArg() < 1 {
		return errors.New("image name needs to be specified")
	}

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

	dataRoot := clicontext.String("data-root")
	if err := os.MkdirAll(dataRoot, 0700); err != nil {
		return err
	}

	ns := clicontext.String("namespace")
	if strings.Contains(ns, "/") {
		return errors.New("namespace with '/' is unsupported")
	}
	stateDir := filepath.Join(dataRoot, "c", ns, id) // "c" stands for "containers"
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

	flagI := clicontext.Bool("i")
	flagT := clicontext.Bool("t")
	flagD := clicontext.Bool("d")

	if flagI {
		if flagD {
			return errors.New("currently flag -t and -d cannot be specified together (FIXME)")
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

	var mounts []specs.Mount
	for _, v := range clicontext.StringSlice("v") {
		m, err := mountutil.ParseFlagV(v)
		if err != nil {
			return err
		}
		mounts = append(mounts, *m)
	}
	opts = append(opts, oci.WithMounts(mounts))

	restartOpts, err := generateRestartOpts(clicontext.String("restart"))
	if err != nil {
		return err
	}
	cOpts = append(cOpts, restartOpts...)

	switch netstr := clicontext.String("network"); netstr {
	case "none":
		// NOP
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.NetworkNamespace), oci.WithHostHostsFile, oci.WithHostResolvconf)
	case "bridge":
		// We only verify flags here.
		// The actual network is configured in the oci hook.
		cniPath := clicontext.String("cni-path")
		for _, f := range requiredCNIPlugins {
			p := filepath.Join(cniPath, f)
			if _, err := exec.LookPath(p); err != nil {
				return errors.Wrapf(err, "needs CNI plugin %q to be installed in CNI_PATH (%q), see https://github.com/containernetworking/plugins/releases",
					f, cniPath)
			}
		}
		for _, dns := range clicontext.StringSlice("dns") {
			if net.ParseIP(dns) == nil {
				return errors.Errorf("invalid dns %q", dns)
			}
		}
		for _, p := range clicontext.StringSlice("p") {
			if _, err = portutil.ParseFlagP(p); err != nil {
				return err
			}
		}
	default:
		return errors.Errorf("unknown network %q", netstr)
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

	if cgOpts, err := generateCgroupOpts(clicontext); err != nil {
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

	if clicontext.Bool("privileged") {
		opts = append(opts, privilegedOpts...)
	}

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)
	cOpts = append(cOpts, spec)

	container, err := client.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return err
	}
	if clicontext.Bool("rm") && !flagD {
		defer func() {
			if removeErr := removeContainer(ctx, client, id, true); removeErr != nil {
				if !errdefs.IsNotFound(removeErr) {
					logrus.WithError(removeErr).Warnf("failed to remove container %s", id)
				}
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

	task, err := tasks.NewTask(ctx, client, container, "", con, flagD, "", nil)
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

func withNerdctlOCIHook(clicontext *cli.Context, id, stateDir string) (oci.SpecOpts, error) {
	selfExe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return nil, err
	}
	fullID := clicontext.String("namespace") + "-" + id
	args := []string{
		// FIXME: How to propagate all global flags?
		"--cni-path=" + clicontext.String("cni-path"),
		"internal",
		"oci-hook",
		"--full-id=" + fullID,
		"--container-state-dir=" + stateDir,
		"--network=" + clicontext.String("network"),
	}
	for _, dns := range clicontext.StringSlice("dns") {
		args = append(args, "--dns="+dns)
	}
	for _, p := range clicontext.StringSlice("p") {
		args = append(args, "-p="+p)
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

func generateRestartOpts(restartFlag string) ([]containerd.NewContainerOpts, error) {
	switch restartFlag {
	case "", "no":
		return nil, nil
	case "always":
		return []containerd.NewContainerOpts{restart.WithStatus(containerd.Running)}, nil
	default:
		return nil, errors.Errorf("unsupported restart type %q, supported types are: \"no\",  \"always\"", restartFlag)
	}
}
