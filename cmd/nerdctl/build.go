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
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newBuildCommand() *cobra.Command {
	var buildCommand = &cobra.Command{
		Use:           "build",
		Short:         "Build an image from a Dockerfile. Needs buildkitd to be running.",
		RunE:          buildAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	buildCommand.Flags().AddFlag(
		&pflag.Flag{
			Name:    "buildkit-host",
			Usage:   `BuildKit address`,
			EnvVars: []string{"BUILDKIT_HOST"},
			Value:   pflag.NewStringValue(defaults.BuildKitHost(), new(string)),
		},
	)
	buildCommand.Flags().StringSliceP("tag", "t", nil, "Name and optionally a tag in the 'name:tag' format")
	buildCommand.Flags().StringP("file", "f", "", "Name of the Dockerfile")
	buildCommand.Flags().String("target", "", "Set the target build stage to build")
	buildCommand.Flags().StringSlice("build-arg", nil, "Set build-time variables")
	buildCommand.Flags().Bool("no-cache", false, "Do not use cache when building the image")
	buildCommand.Flags().StringP("output", "o", "", "Output destination (format: type=local,dest=path)")
	buildCommand.Flags().String("progress", "auto", "Set type of progress output (auto, plain, tty). Use plain to show container output")
	buildCommand.Flags().StringSlice("secret", nil, "Secret file to expose to the build: id=mysecret,src=/local/secret")
	buildCommand.Flags().StringSlice("ssh", nil, "SSH agent socket or keys to expose to the build (format: default|<id>[=<socket>|<key>[,<key>]])")
	buildCommand.Flags().BoolP("quiet", "q", false, "Suppress the build output and print image ID on success")
	buildCommand.Flags().StringSlice("cache-from", nil, "External cache sources (eg. user/app:cache, type=local,src=path/to/dir)")
	buildCommand.Flags().StringSlice("cache-to", nil, "Cache export destinations (eg. user/app:cache, type=local,dest=path/to/dir)")

	return buildCommand
}

func buildAction(cmd *cobra.Command, args []string) error {
	buildkitHost, err := cmd.Flags().GetString("buildkit-host")
	if err != nil {
		return err
	}
	if err := buildkitutil.PingBKDaemon(buildkitHost); err != nil {
		return err
	}

	buildctlBinary, buildctlArgs, needsLoading, cleanup, err := generateBuildctlArgs(cmd, args)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}

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
		buildctlCmd.Stdout = cmd.OutOrStdout()
	}
	if !quiet {
		buildctlCmd.Stderr = cmd.ErrOrStderr()
	}

	if err := buildctlCmd.Start(); err != nil {
		return err
	}

	if needsLoading {
		if err = loadImage(buildctlStdout, cmd, args, false, quiet); err != nil {
			return err
		}
	}

	if err = buildctlCmd.Wait(); err != nil {
		return err
	}

	return nil
}

func generateBuildctlArgs(cmd *cobra.Command, args []string) (string, []string, bool, func(), error) {
	var needsLoading bool
	if len(args) < 1 {
		return "", nil, false, nil, errors.New("context needs to be specified")
	}
	buildContext := args[0]
	if buildContext == "-" || strings.Contains(buildContext, "://") {
		return "", nil, false, nil, errors.Errorf("unsupported build context: %q", buildContext)
	}

	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		return "", nil, false, nil, err
	}

	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", nil, false, nil, err
	}
	if output == "" {
		output = "type=docker"
		needsLoading = true
	}
	tagValue, err := cmd.Flags().GetStringSlice("tag")
	if err != nil {
		return "", nil, false, nil, err
	}
	if tagSlice := strutil.DedupeStrSlice(tagValue); len(tagSlice) > 0 {
		if len(tagSlice) > 1 {
			return "", nil, false, nil, errors.Errorf("specifying multiple -t is not supported yet")
		}
		output += ",name=" + tagSlice[0]
	}

	buildkitHost, err := cmd.Flags().GetString("buildkit-host")
	if err != nil {
		return "", nil, false, nil, err
	}

	buildctlArgs := buildkitutil.BuildctlBaseArgs(buildkitHost)

	progressValue, err := cmd.Flags().GetString("progress")
	if err != nil {
		return "", nil, false, nil, err
	}

	buildctlArgs = append(buildctlArgs, []string{
		"build",
		"--progress=" + progressValue,
		"--frontend=dockerfile.v0",
		"--local=context=" + buildContext,
		"--local=dockerfile=" + buildContext,
		"--output=" + output,
	}...)

	filename, err := cmd.Flags().GetString("file")
	if err != nil {
		return "", nil, false, nil, err
	}

	var dir, file string
	var cleanup func()

	if filename != "" {
		if filename == "-" {
			var err error
			dir, err = buildkitutil.WriteTempDockerfile(cmd.InOrStdin())
			if err != nil {
				return "", nil, false, nil, err
			}
			file = buildkitutil.DefaultDockerfileName
			cleanup = func() {
				os.RemoveAll(dir)
			}
		} else {
			dir, file = filepath.Split(filename)
		}

		if dir != "" {
			buildctlArgs = append(buildctlArgs, "--local=dockerfile="+dir)
		}
		buildctlArgs = append(buildctlArgs, "--opt=filename="+file)
	}

	target, err := cmd.Flags().GetString("target")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	if target != "" {
		buildctlArgs = append(buildctlArgs, "--opt=target="+target)
	}

	buildArgsValue, err := cmd.Flags().GetStringSlice("build-arg")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, ba := range strutil.DedupeStrSlice(buildArgsValue) {
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

	noCache, err := cmd.Flags().GetBool("no-cache")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	if noCache {
		buildctlArgs = append(buildctlArgs, "--no-cache")
	}

	secretValue, err := cmd.Flags().GetStringSlice("secret")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(secretValue) {
		buildctlArgs = append(buildctlArgs, "--secret="+s)
	}

	sshValue, err := cmd.Flags().GetStringSlice("ssh")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(sshValue) {
		buildctlArgs = append(buildctlArgs, "--ssh="+s)
	}

	cacheFrom, err := cmd.Flags().GetStringSlice("cache-from")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(cacheFrom) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--import-cache="+s)
	}

	cacheTo, err := cmd.Flags().GetStringSlice("cache-to")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(cacheTo) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--export-cache="+s)
	}

	return buildctlBinary, buildctlArgs, needsLoading, cleanup, nil
}
