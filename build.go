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
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/containerd/containerd"
	"github.com/urfave/cli/v2"
)

var buildCommand = &cli.Command{
	Name:   "build",
	Usage:  "Build an image from a Dockerfile. Needs buildkitd to be running.",
	Action: buildAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "tag",
			Aliases: []string{"t"},
			Usage:   "Name and optionally a tag in the 'name:tag' format",
		},
	},
}

func buildAction(clicontext *cli.Context) error {
	if clicontext.NArg() < 1 {
		return errors.New("context needs to be specified")
	}
	buildContext := clicontext.Args().First()
	tag := clicontext.String("tag")
	if tag == "" {
		return errors.New("tag needs to be specified")
	}
	sn := containerd.DefaultSnapshotter
	containerdAddr := strings.TrimPrefix(clicontext.String("host"), "unix://")
	ns := clicontext.String("namespace")

	buildctlCmd := exec.Command("buildctl",
		"build",
		"--frontend=dockerfile.v0",
		"--local=context="+buildContext,
		"--local=dockerfile="+buildContext,
		"--output=type=docker,name="+tag)
	buildctlCmd.Env = os.Environ()

	// FIXME: do not rely on ctr command
	ctrCmd := exec.Command("ctr",
		"--address", containerdAddr,
		"--namespace", ns,
		"images",
		"import",
		"--snapshotter", sn,
		"-")
	ctrCmd.Env = os.Environ()

	buildctlStdout, err := buildctlCmd.StdoutPipe()
	if err != nil {
		return err
	}
	buildctlCmd.Stderr = clicontext.App.Writer
	ctrCmd.Stdin = buildctlStdout
	ctrCmd.Stdout = clicontext.App.Writer
	ctrCmd.Stderr = clicontext.App.ErrWriter

	if err := buildctlCmd.Start(); err != nil {
		return err
	}
	if err := ctrCmd.Start(); err != nil {
		return err
	}
	return ctrCmd.Wait()
}
