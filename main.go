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

	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/namespaces"
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
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:        "debug",
			Usage:       "debug mode",
			Destination: &debug,
		},
		&cli.StringFlag{
			Name:    "host",
			Aliases: []string{"H"},
			Usage:   "containerd address, e.g. \"unix:///run/containerd/containerd.sock\", or \"/var/run/docker/containerd/containerd.sock\"",
			Value:   "unix://" + defaults.DefaultAddress,
		},
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "containerd namespace, such as \"moby\" for Docker, \"k8s.io\" for Kubernetes",
			EnvVars: []string{namespaces.NamespaceEnvVar},
			Value:   namespaces.Default,
		},
	}
	app.Before = func(context *cli.Context) error {
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.Commands = []*cli.Command{
		buildCommand,
		imagesCommand,
		psCommand,
		rmCommand,
		pullCommand,
		runCommand,
	}
	return app
}
