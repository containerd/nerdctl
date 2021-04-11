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
	"os"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/namespaces"
	ncdefaults "github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type Category = string

const (
	CategoryManagement = Category("Management")
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
	app := cli.NewApp()
	app.Name = "nerdctl"
	app.Usage = "Docker-compatible CLI for containerd"
	app.UseShortOptionHandling = true
	app.EnableBashCompletion = true
	app.Version = strings.TrimPrefix(version.Version, "v")
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "debug mode",
		},
		&cli.BoolFlag{
			Name:  "debug-full",
			Usage: "debug mode (with full output)",
		},
		&cli.StringFlag{
			Name:    "address",
			Aliases: []string{"a", "host", "H"},
			Usage:   "containerd address, optionally with \"unix://\" prefix",
			EnvVars: []string{"CONTAINERD_ADDRESS"},
			Value:   defaults.DefaultAddress, // same for rootless as well (mount-namespaced)
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
		if clicontext.Bool("debug-full") {
			clicontext.Set("debug", "true")
		}
		if clicontext.Bool("debug") {
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
		waitCommand,
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
		// Compose
		composeCommand,
		// Completion
		completionCommand,
	}
	app.BashComplete = appBashComplete
	return app
}

func globalFlags(clicontext *cli.Context) (string, []string) {
	args0, err := os.Executable()
	if err != nil {
		logrus.WithError(err).Warnf("cannot call os.Executable(), assuming the executable to be %q", os.Args[0])
		args0 = os.Args[0]
	}
	if len(os.Args) < 2 {
		return args0, nil
	}
	lineage := clicontext.Lineage()
	top := lineage[len(lineage)-2]
	return args0, strutil.TrimStrSliceRight(os.Args[1:], top.Args().Slice())
}
