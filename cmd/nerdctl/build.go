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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"path/filepath"

	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/strutil"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newBuildCommand() *cobra.Command {
	var buildCommand = &cobra.Command{
		Use:           "build",
		Short:         "Build an image from a Dockerfile. Needs buildkitd to be running.",
		RunE:          buildAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	AddStringFlag(buildCommand, "buildkit-host", nil, defaults.BuildKitHost(), "BUILDKIT_HOST", "BuildKit address")
	buildCommand.Flags().StringArrayP("tag", "t", nil, "Name and optionally a tag in the 'name:tag' format")
	buildCommand.Flags().StringP("file", "f", "", "Name of the Dockerfile")
	buildCommand.Flags().String("target", "", "Set the target build stage to build")
	buildCommand.Flags().StringArray("build-arg", nil, "Set build-time variables")
	buildCommand.Flags().Bool("no-cache", false, "Do not use cache when building the image")
	buildCommand.Flags().StringP("output", "o", "", "Output destination (format: type=local,dest=path)")
	buildCommand.Flags().String("progress", "auto", "Set type of progress output (auto, plain, tty). Use plain to show container output")
	buildCommand.Flags().StringArray("secret", nil, "Secret file to expose to the build: id=mysecret,src=/local/secret")
	buildCommand.Flags().StringArray("ssh", nil, "SSH agent socket or keys to expose to the build (format: default|<id>[=<socket>|<key>[,<key>]])")
	buildCommand.Flags().BoolP("quiet", "q", false, "Suppress the build output and print image ID on success")
	buildCommand.Flags().StringArray("cache-from", nil, "External cache sources (eg. user/app:cache, type=local,src=path/to/dir)")
	buildCommand.Flags().StringArray("cache-to", nil, "Cache export destinations (eg. user/app:cache, type=local,dest=path/to/dir)")

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	buildCommand.Flags().StringSlice("platform", []string{}, "Set target platform for build (e.g., \"amd64\", \"arm64\")")
	buildCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	// #endregion

	buildCommand.Flags().Bool("ipfs", false, "Allow pulling base images from IPFS")

	return buildCommand
}

func buildAction(cmd *cobra.Command, args []string) error {
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	platform = strutil.DedupeStrSlice(platform)

	buildkitHost, err := cmd.Flags().GetString("buildkit-host")
	if err != nil {
		return err
	}
	if err := buildkitutil.PingBKDaemon(buildkitHost); err != nil {
		return err
	}

	buildctlBinary, buildctlArgs, needsLoading, cleanup, err := generateBuildctlArgs(cmd, platform, args)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	runIPFSRegistry, err := cmd.Flags().GetBool("ipfs")
	if err != nil {
		return err
	}
	if runIPFSRegistry {
		logrus.Infof("Ensuring IPFS registry is running")
		nerdctlCmd, nerdctlArgs := globalFlags(cmd)
		if out, err := exec.Command(nerdctlCmd, append(nerdctlArgs, "ipfs", "registry", "up")...).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to start IPFS registry: %v: %v", string(out), err)
		} else {
			logrus.Infof("IPFS registry is running: %v", string(out))
		}
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
		platMC, err := platformutil.NewMatchComparer(false, platform)
		if err != nil {
			return err
		}
		if _, err = loadImage(buildctlStdout, cmd, args, platMC, quiet); err != nil {
			return err
		}
	}

	if err = buildctlCmd.Wait(); err != nil {
		return err
	}

	return nil
}

func generateBuildctlArgs(cmd *cobra.Command, platform, args []string) (string, []string, bool, func(), error) {
	var needsLoading bool
	if len(args) < 1 {
		return "", nil, false, nil, errors.New("context needs to be specified")
	}
	buildContext := args[0]
	if buildContext == "-" || strings.Contains(buildContext, "://") {
		return "", nil, false, nil, fmt.Errorf("unsupported build context: %q", buildContext)
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
		if len(platform) > 1 {
			// For avoiding `error: failed to solve: docker exporter does not currently support exporting manifest lists`
			// TODO: consider using type=oci for single-platform build too
			output = "type=oci"
		}
		needsLoading = true
	}
	tagValue, err := cmd.Flags().GetStringArray("tag")
	if err != nil {
		return "", nil, false, nil, err
	}
	if tagSlice := strutil.DedupeStrSlice(tagValue); len(tagSlice) > 0 {
		if len(tagSlice) > 1 {
			return "", nil, false, nil, fmt.Errorf("specifying multiple -t is not supported yet")
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

	if len(platform) > 0 {
		buildctlArgs = append(buildctlArgs, "--opt=platform="+strings.Join(platform, ","))
	}

	buildArgsValue, err := cmd.Flags().GetStringArray("build-arg")
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

	secretValue, err := cmd.Flags().GetStringArray("secret")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(secretValue) {
		buildctlArgs = append(buildctlArgs, "--secret="+s)
	}

	sshValue, err := cmd.Flags().GetStringArray("ssh")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(sshValue) {
		buildctlArgs = append(buildctlArgs, "--ssh="+s)
	}

	cacheFrom, err := cmd.Flags().GetStringArray("cache-from")
	if err != nil {
		return "", nil, false, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(cacheFrom) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--import-cache="+s)
	}

	cacheTo, err := cmd.Flags().GetStringArray("cache-to")
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
