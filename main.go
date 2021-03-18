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
	"os"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/namespaces"
	ncdefaults "github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	if err := xmain(); err != nil {
		logrus.Fatal(err)
	}
}

func xmain() error {
	if len(os.Args) == 3 && os.Args[1] == logging.MagicArgv1 {
		// containerd runtime v2 logging plugin mode.
		// "binary://BIN?KEY=VALUE" URI is parsed into Args {BIN, KEY, VALUE}.
		return logging.Main(os.Args[2])
	}
	// nerdctl CLI mode
	return newApp().Run(os.Args)
}

func newApp() *cli.App {
	debug := false
	app := cli.NewApp()
	app.Name = "nerdctl"
	app.Usage = "Docker-compatible CLI for containerd"
	app.UseShortOptionHandling = true
	app.EnableBashCompletion = true
	app.Version = strings.TrimPrefix(version.Version, "v")
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:        "debug",
			Usage:       "debug mode",
			Destination: &debug,
		},
		&cli.StringFlag{
			Name:    "address",
			Aliases: []string{"a", "host", "H"},
			Usage:   "containerd address, optionally with \"unix://\" prefix",
			EnvVars: []string{"CONTAINERD_ADDRESS"},
			Value:   "unix://" + defaults.DefaultAddress, // same for rootless as well (mount-namespaced)
		},
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "containerd namespace, such as \"moby\" for Docker, \"k8s.io\" for Kubernetes",
			EnvVars: []string{namespaces.NamespaceEnvVar},
			Value:   namespaces.Default,
		},
		&cli.StringFlag{
			Name:    "snapshotter",
			Aliases: []string{"storage-driver"},
			Usage:   "containerd snapshotter",
			EnvVars: []string{"CONTAINERD_SNAPSHOTTER"},
			// TODO: change the default to "native" when overlayfs is unavailable (on rootless)
			Value: containerd.DefaultSnapshotter,
		},
		&cli.StringFlag{
			Name:  "cni-path",
			Usage: "Set the cni-plugins binary directory",
			// CNI_PATH is from https://www.cni.dev/docs/cnitool/
			EnvVars: []string{"CNI_PATH"},
			Value:   ncdefaults.CNIPath(),
		},
		&cli.StringFlag{
			Name:  "cni-netconfpath",
			Usage: "Set the CNI config directory",
			// NETCONFPATH is from https://www.cni.dev/docs/cnitool/
			EnvVars: []string{"NETCONFPATH"},
			Value:   ncdefaults.CNINetConfPath(),
		},
		&cli.StringFlag{
			Name:  "data-root",
			Usage: "Root directory of persistent nerdctl state (managed by nerdctl, not by containerd)",
			Value: ncdefaults.DataRoot(),
		},
		// cgroup-manager flag is from Podman.
		// Because Docker's equivalent is complicated: --exec-opt native.cgroupdriver=(cgroupfs|systemd)
		&cli.StringFlag{
			Name:  "cgroup-manager",
			Usage: "Cgroup manager to use (\"cgroupfs\"|\"systemd\")",
			Value: ncdefaults.CgroupManager(),
		},
		&cli.BoolFlag{
			Name:  "insecure-registry",
			Usage: "skips verifying HTTPS certs, and allows falling back to plain HTTP",
		},
	}
	app.Before = func(clicontext *cli.Context) error {
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}

		address := clicontext.String("address")
		if strings.Contains(address, "://") && !strings.HasPrefix(address, "unix://") {
			return errors.Errorf("invalid address %q", address)
		}
		if appNeedsRootlessParentMain(clicontext) {
			// reexec /proc/self/exe with `nsenter` into RootlessKit namespaces
			return rootlessutil.ParentMain()
		}
		return nil
	}
	app.Commands = []*cli.Command{
		// Run & Exec
		runCommand,
		execCommand,
		// Container management
		psCommand,
		logsCommand,
		portCommand,
		stopCommand,
		startCommand,
		killCommand,
		rmCommand,
		pauseCommand,
		unpauseCommand,
		commitCommand,
		// Build
		buildCommand,
		// Image management
		imagesCommand,
		pullCommand,
		pushCommand,
		loadCommand,
		saveCommand,
		tagCommand,
		rmiCommand,
		// System
		eventsCommand,
		infoCommand,
		versionCommand,
		// Inspect
		inspectCommand,
		// Management
		containerCommand,
		imageCommand,
		networkCommand,
		volumeCommand,
		systemCommand,
		namespaceCommand,
		// Internal
		internalCommand,
		// login
		loginCommand,
		// Logout
		logoutCommand,
		// Completion
		completionCommand,
	}
	app.BashComplete = appBashComplete
	return app
}

func appNeedsRootlessParentMain(clicontext *cli.Context) bool {
	if !rootlessutil.IsRootlessParent() {
		return false
	}
	// TODO: allow `nerdctl <SUBCOMMAND> --help` without nsentering into RootlessKit
	switch clicontext.Args().First() {
	case "", "completion", "login", "logout":
		return false
	}
	return true
}

type Category = string

const (
	CategoryManagement = Category("Management")
)

func appBashComplete(clicontext *cli.Context) {
	w := clicontext.App.Writer
	coco := parseCompletionContext(clicontext)
	switch coco.flagName {
	case "n", "namespace":
		bashCompleteNamespaceNames(clicontext)
		return
	case "snapshotter", "storage-driver":
		bashCompleteSnapshotterNames(clicontext)
		return
	case "cgroup-manager":
		fmt.Fprintln(w, "cgroupfs")
		if ncdefaults.IsSystemdAvailable() {
			fmt.Fprintln(w, "systemd")
		}
		if rootlessutil.IsRootless() {
			fmt.Fprintln(w, "none")
		}
		return
	}
	cli.DefaultAppComplete(clicontext)
	for _, subcomm := range clicontext.App.Commands {
		fmt.Fprintln(clicontext.App.Writer, subcomm.Name)
	}
}

func bashCompleteNamespaceNames(clicontext *cli.Context) {
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain()
		return
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return
	}
	defer cancel()
	nsService := client.NamespaceService()
	nsList, err := nsService.List(ctx)
	if err != nil {
		logrus.Warn(err)
		return
	}
	for _, ns := range nsList {
		fmt.Fprintln(clicontext.App.Writer, ns)
	}
}

func bashCompleteSnapshotterNames(clicontext *cli.Context) {
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain()
		return
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return
	}
	defer cancel()
	snapshotterPlugins, err := getSnapshotterNames(ctx, client.IntrospectionService())
	if err != nil {
		return
	}
	for _, name := range snapshotterPlugins {
		fmt.Fprintln(clicontext.App.Writer, name)
	}
}
