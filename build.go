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
	"fmt"
	"os"
	"os/exec"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images/archive"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
	sn := clicontext.String("snapshotter")

	buildctlCheckCmd := exec.Command("buildctl", "debug", "workers")
	buildctlCheckCmd.Env = os.Environ()
	if out, err := buildctlCheckCmd.CombinedOutput(); err != nil {
		logrus.Error(string(out))
		return errors.Wrap(err, "`buildctl` needs to be installed and `buildkitd` needs to be running, see https://github.com/moby/buildkit")
	}

	buildctlCmd := exec.Command("buildctl",
		"build",
		"--frontend=dockerfile.v0",
		"--local=context="+buildContext,
		"--local=dockerfile="+buildContext,
		"--output=type=docker,name="+tag)
	buildctlCmd.Env = os.Environ()

	buildctlStdout, err := buildctlCmd.StdoutPipe()
	if err != nil {
		return err
	}
	buildctlCmd.Stderr = clicontext.App.ErrWriter

	if err := buildctlCmd.Start(); err != nil {
		return err
	}

	// Load images from buildkit tar stream
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	var opts []containerd.ImportOpt
	opts = append(opts, containerd.WithDigestRef(archive.DigestTranslator(sn)))

	imgs, err := client.Import(ctx, buildctlStdout, opts...)
	if err != nil {
		return err
	}

	// Wait exporting images to containerd
	if err = buildctlCmd.Wait(); err != nil {
		return err
	}

	for _, img := range imgs {
		image := containerd.NewImage(client, img)

		// TODO: Show unpack status
		fmt.Fprintf(clicontext.App.Writer, "unpacking %s (%s)...", img.Name, img.Target.Digest)
		err = image.Unpack(ctx, sn)
		if err != nil {
			return err
		}
		fmt.Fprintf(clicontext.App.Writer, "done\n")
	}

	return nil
}
