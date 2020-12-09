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
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	gocni "github.com/containerd/go-cni"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var runCommand = &cli.Command{
	Name:   "run",
	Usage:  "Run a command in a new container",
	Action: runAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "tty",
			Aliases: []string{"t"},
			Usage:   "(Currently always needs to be true)",
		},
		&cli.BoolFlag{
			Name:    "interactive",
			Aliases: []string{"i"},
			Usage:   "(Currently always needs to be true)",
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
			Name:  "security-opt",
			Usage: "Security options",
		},
		&cli.BoolFlag{
			Name:  "privileged",
			Usage: "Give extended privileges to this container",
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
	ensured, err := ensureImage(ctx, client, clicontext.App.Writer, clicontext.String("snapshotter"), clicontext.Args().First(), clicontext.String("pull"))
	if err != nil {
		return err
	}
	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		id    = genID()
	)
	opts = append(opts,
		oci.WithDefaultSpec(),
		oci.WithDefaultUnixDevices,
		oci.WithImageConfig(ensured.Image),
	)
	cOpts = append(cOpts,
		containerd.WithImage(ensured.Image),
		containerd.WithSnapshotter(ensured.Snapshotter),
		containerd.WithNewSnapshot(id, ensured.Image),
		containerd.WithImageStopSignal(ensured.Image, "SIGTERM"),
	)
	if clicontext.NArg() > 1 {
		opts = append(opts, oci.WithProcessArgs(clicontext.Args().Tail()...))
	}

	if !clicontext.Bool("i") || !clicontext.Bool("t") {
		return errors.New("currently -i and -t need to be always specified (FIXME)")
	}
	opts = append(opts, oci.WithTTY)

	var cniNetwork gocni.CNI
	switch netstr := clicontext.String("network"); netstr {
	case "none":
		// NOP
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.NetworkNamespace), oci.WithHostHostsFile, oci.WithHostResolvconf)
	case "bridge":
		for _, f := range requiredCNIPlugins {
			p := filepath.Join(gocni.DefaultCNIDir, f)
			if _, err := exec.LookPath(p); err != nil {
				return errors.Wrapf(err, "needs CNI plugin %q to be installed, see https://github.com/containernetworking/plugins/releases", p)
			}
		}
		if cniNetwork, err = gocni.New(gocni.WithConfListBytes([]byte(defaultBridgeNetwork))); err != nil {
			return err
		}
		resolvConf, err := ioutil.TempFile("", "nerdctl-resolvconf")
		if err != nil {
			return err
		}
		defer os.RemoveAll(resolvConf.Name())
		if _, err = resolvConf.Write([]byte("search localdomain\n")); err != nil {
			return err
		}
		for _, dns := range clicontext.StringSlice("dns") {
			if net.ParseIP(dns) == nil {
				return errors.Errorf("invalid dns %q", dns)
			}
			if _, err = resolvConf.Write([]byte("nameserver " + dns + "\n")); err != nil {
				return err
			}
		}
		opts = append(opts, withCustomResolvConf(resolvConf.Name()))
	default:
		return errors.Errorf("unknown network %q", netstr)
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
	if clicontext.Bool("rm") {
		defer container.Delete(ctx, containerd.WithSnapshotCleanup)
	}

	con := console.Current()
	defer con.Reset()
	if err := con.SetRaw(); err != nil {
		return err
	}

	task, err := tasks.NewTask(ctx, client, container, "", con, false, "", nil)
	if err != nil {
		return err
	}
	defer func() {
		if cniNetwork != nil {
			if err := cniNetwork.Remove(ctx, fullID(ctx, container), ""); err != nil {
				logrus.WithError(err).Error("network review")
			}
		}
		task.Delete(ctx)
	}()
	statusC, err := task.Wait(ctx)
	if err != nil {
		return err
	}
	if cniNetwork != nil {
		if _, err := cniNetwork.Setup(ctx, fullID(ctx, container), fmt.Sprintf("/proc/%d/ns/net", task.Pid())); err != nil {
			return err
		}
	}
	if err := task.Start(ctx); err != nil {
		return err
	}
	if err := tasks.HandleConsoleResize(ctx, task, con); err != nil {
		logrus.WithError(err).Error("console resize")
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

// fullID is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run.go
func fullID(ctx context.Context, c containerd.Container) string {
	id := c.ID()
	ns, ok := namespaces.Namespace(ctx)
	if !ok {
		return id
	}
	return fmt.Sprintf("%s-%s", ns, id)
}

func withCustomResolvConf(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      src,
			Options:     []string{"rbind", "ro"},
		})
		return nil
	}
}
