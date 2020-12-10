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
	"os"
	"strings"

	"github.com/AkihiroSuda/nerdctl/pkg/version"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/namespaces"
	gocni "github.com/containerd/go-cni"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	if err := newApp().Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func newApp() *cli.App {
	debug := false
	app := cli.NewApp()
	app.Name = "nerdctl"
	app.Usage = "Docker-compatible CLI for containerd"
	app.UseShortOptionHandling = true
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
			Value:   "unix://" + defaults.DefaultAddress,
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
			Value:   containerd.DefaultSnapshotter,
		},
		&cli.StringFlag{
			Name:    "cni-path",
			Usage:   "Set the cni-plugins binary directory",
			EnvVars: []string{"CNI_PATH"},
			Value:   gocni.DefaultCNIDir,
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
		return nil
	}
	app.Commands = []*cli.Command{
		buildCommand,
		imagesCommand,
		infoCommand,
		psCommand,
		rmCommand,
		rmiCommand,
		pullCommand,
		runCommand,
		versionCommand,
	}
	return app
}
