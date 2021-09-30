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
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"path/filepath"

	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/strutil"
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
			Name:    "buildkit-host",
			Usage:   "BuildKit address",
			EnvVars: []string{"BUILDKIT_HOST"},
			Value:   defaults.BuildKitHost(),
		},

		&cli.StringSliceFlag{
			Name:    "tag",
			Aliases: []string{"t"},
			Usage:   "Name and optionally a tag in the 'name:tag' format",
		},
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Name of the Dockerfile",
		},
		&cli.StringFlag{
			Name:  "target",
			Usage: "Set the target build stage to build",
		},
		&cli.StringSliceFlag{
			Name:  "build-arg",
			Usage: "Set build-time variables",
		},
		&cli.BoolFlag{
			Name:  "no-cache",
			Usage: "Do not use cache when building the image",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Output destination (format: type=local,dest=path)",
		},
		&cli.StringFlag{
			Name:  "progress",
			Usage: "Set type of progress output (auto, plain, tty). Use plain to show container output",
			Value: "auto",
		},
		&cli.StringSliceFlag{
			Name:  "secret",
			Usage: "Secret file to expose to the build: id=mysecret,src=/local/secret",
		},
		&cli.StringSliceFlag{
			Name:  "ssh",
			Usage: "SSH agent socket or keys to expose to the build (format: default|<id>[=<socket>|<key>[,<key>]])",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Suppress the build output and print image ID on success",
		},
		&cli.StringSliceFlag{
			Name:  "cache-from",
			Usage: "External cache sources (eg. user/app:cache, type=local,src=path/to/dir)",
		},
		&cli.StringSliceFlag{
			Name:  "cache-to",
			Usage: "Cache export destinations (eg. user/app:cache, type=local,dest=path/to/dir)",
		},
	},
}

func buildAction(clicontext *cli.Context) error {
	buildkitHost := clicontext.String("buildkit-host")
	if err := buildkitutil.PingBKDaemon(buildkitHost); err != nil {
		return err
	}

	buildctlBinary, buildctlArgs, needsLoading, err := generateBuildctlArgs(clicontext)
	if err != nil {
		return err
	}

	quiet := clicontext.Bool("quiet")

	logrus.Debugf("running %s %v", buildctlBinary, buildctlArgs)
	buildctlCmd := exec.Command(buildctlBinary, buildctlArgs...)
	buildctlCmd.Env = os.Environ()

	var buildctlStdout io.Reader
	if needsLoading {
		buildctlStdout, err = buildctlCmd.StdoutPipe()
		if err != nil {
			return err
		}
	} else {
		buildctlCmd.Stdout = clicontext.App.Writer
	}

	if !quiet {
		buildctlCmd.Stderr = clicontext.App.ErrWriter
	}

	if err := buildctlCmd.Start(); err != nil {
		return err
	}

	if needsLoading {
		if err = loadImage(buildctlStdout, clicontext, quiet); err != nil {
			return err
		}
	}

	if err = buildctlCmd.Wait(); err != nil {
		return err
	}

	return nil
}

func generateBuildctlArgs(clicontext *cli.Context) (string, []string, bool, error) {
	var needsLoading bool
	if clicontext.NArg() < 1 {
		return "", nil, false, errors.New("context needs to be specified")
	}
	buildContext := clicontext.Args().First()
	if buildContext == "-" || strings.Contains(buildContext, "://") {
		return "", nil, false, errors.Errorf("unsupported build context: %q", buildContext)
	}

	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		return "", nil, false, err
	}

	output := clicontext.String("output")
	if output == "" {
		output = "type=docker"
		needsLoading = true
	}
	if tagSlice := strutil.DedupeStrSlice(clicontext.StringSlice("tag")); len(tagSlice) > 0 {
		if len(tagSlice) > 1 {
			return "", nil, false, errors.Errorf("specifying multiple -t is not supported yet")
		}
		output += ",name=" + tagSlice[0]
	}

	buildctlArgs := buildkitutil.BuildctlBaseArgs(clicontext.String("buildkit-host"))

	buildctlArgs = append(buildctlArgs, []string{
		"build",
		"--progress=" + clicontext.String("progress"),
		"--frontend=dockerfile.v0",
		"--local=context=" + buildContext,
		"--local=dockerfile=" + buildContext,
		"--output=" + output,
	}...)

	if filename := clicontext.String("file"); filename != "" {
		dir, file := filepath.Split(filename)
		if dir != "" {
			buildctlArgs = append(buildctlArgs, "--local=dockerfile="+dir)
		}
		buildctlArgs = append(buildctlArgs, "--opt=filename="+file)
	}

	if target := clicontext.String("target"); target != "" {
		buildctlArgs = append(buildctlArgs, "--opt=target="+target)
	}

	for _, ba := range strutil.DedupeStrSlice(clicontext.StringSlice("build-arg")) {
		buildctlArgs = append(buildctlArgs, "--opt=build-arg:"+ba)

		// Support `--build-arg BUILDKIT_INLINE_CACHE=1` for compatibility with `docker buildx build`
		// https://github.com/docker/buildx/blob/v0.6.3/docs/reference/buildx_build.md#-export-build-cache-to-an-external-cache-destination---cache-to
		if strings.HasPrefix(ba, "BUILDKIT_INLINE_CACHE=") {
			bic := strings.TrimPrefix(ba, "BUILDKIT_INLINE_CACHE=")
			bicParsed, err := strconv.ParseBool(bic)
			if err == nil {
				if bicParsed {
					buildctlArgs = append(buildctlArgs, "--export-cache=type=inline")
				}
			} else {
				logrus.WithError(err).Warnf("invalid BUILDKIT_INLINE_CACHE: %q", bic)
			}
		}
	}

	if clicontext.Bool("no-cache") {
		buildctlArgs = append(buildctlArgs, "--no-cache")
	}

	for _, s := range strutil.DedupeStrSlice(clicontext.StringSlice("secret")) {
		buildctlArgs = append(buildctlArgs, "--secret="+s)
	}

	for _, s := range strutil.DedupeStrSlice(clicontext.StringSlice("ssh")) {
		buildctlArgs = append(buildctlArgs, "--ssh="+s)
	}

	for _, s := range strutil.DedupeStrSlice(clicontext.StringSlice("cache-from")) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--import-cache="+s)
	}

	for _, s := range strutil.DedupeStrSlice(clicontext.StringSlice("cache-to")) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--export-cache="+s)
	}

	return buildctlBinary, buildctlArgs, needsLoading, nil
}
