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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/errdefs"
	dockerreference "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/strutil"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newBuildCommand(cfg *types.BuildConfig) *cobra.Command {
	var buildCommand = &cobra.Command{
		Use:   "build [flags] PATH",
		Short: "Build an image from a Dockerfile. Needs buildkitd to be running.",
		Long: `Build an image from a Dockerfile. Needs buildkitd to be running.
If Dockerfile is not present and -f is not specified, it will look for Containerfile and build with it. `,
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
	buildCommand.Flags().Bool("rm", true, "Remove intermediate containers after a successful build")

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	buildCommand.Flags().StringSlice("platform", cfg.Platforms, "Set target platform for build (e.g., \"amd64\", \"arm64\")")
	buildCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	// #endregion

	buildCommand.Flags().Bool("ipfs", false, "Allow pulling base images from IPFS")
	buildCommand.Flags().String("iidfile", "", "Write the image ID to the file")
	buildCommand.Flags().StringArray("label", nil, "Set metadata for an image")

	return buildCommand
}

func getBuildkitHost(cmd *cobra.Command) (string, error) {
	if cmd.Flags().Changed("buildkit-host") || os.Getenv("BUILDKIT_HOST") != "" {
		// If address is explicitly specified, use it.
		buildkitHost, err := cmd.Flags().GetString("buildkit-host")
		if err != nil {
			return "", err
		}
		if err := buildkitutil.PingBKDaemon(buildkitHost); err != nil {
			return "", err
		}
		return buildkitHost, nil
	}
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return "", err
	}
	return buildkitutil.GetBuildkitHost(ns)
}

func isImageSharable(buildkitHost string, namespace, uuid, snapshotter string, platform []string) (bool, error) {
	labels, err := buildkitutil.GetWorkerLabels(buildkitHost)
	if err != nil {
		return false, err
	}
	logrus.Debugf("worker labels: %+v", labels)
	executor, ok := labels["org.mobyproject.buildkit.worker.executor"]
	if !ok {
		return false, nil
	}
	containerdUUID, ok := labels["org.mobyproject.buildkit.worker.containerd.uuid"]
	if !ok {
		return false, nil
	}
	containerdNamespace, ok := labels["org.mobyproject.buildkit.worker.containerd.namespace"]
	if !ok {
		return false, nil
	}
	workerSnapshotter, ok := labels["org.mobyproject.buildkit.worker.snapshotter"]
	if !ok {
		return false, nil
	}
	// NOTE: It's possible that BuildKit doesn't download the base image of non-default platform (e.g. when the provided
	//       Dockerfile doesn't contain instructions require base images like RUN) even if `--output type=image,unpack=true`
	//       is passed to BuildKit. Thus, we need to use `type=docker` or `type=oci` when nerdctl builds non-default platform
	//       image using `platform` option.
	return executor == "containerd" && containerdUUID == uuid && containerdNamespace == namespace && workerSnapshotter == snapshotter && len(platform) == 0, nil
}

func buildAction(cmd *cobra.Command, args []string) error {
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	platform = strutil.DedupeStrSlice(platform)

	buildkitHost, err := getBuildkitHost(cmd)
	if err != nil {
		return err
	}

	buildctlBinary, buildctlArgs, needsLoading, metaFile, tags, cleanup, err := generateBuildctlArgs(cmd, buildkitHost, platform, args)
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
		out, err := exec.Command(nerdctlCmd, append(nerdctlArgs, "ipfs", "registry", "up")...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to start IPFS registry: %v: %v", string(out), err)
		}
		logrus.Infof("IPFS registry is running: %v", string(out))
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
		if err = loadImage(buildctlStdout, cmd, platMC, quiet); err != nil {
			return err
		}
	}

	if err = buildctlCmd.Wait(); err != nil {
		return err
	}

	iidFile, _ := cmd.Flags().GetString("iidfile")
	if iidFile != "" {
		id, err := getDigestFromMetaFile(metaFile)
		if err != nil {
			return err
		}
		if err := os.WriteFile(iidFile, []byte(id), 0600); err != nil {
			return err
		}
	}

	if len(tags) > 1 {
		logrus.Debug("Found more than 1 tag")
		client, ctx, cancel, err := newClient(cmd)
		if err != nil {
			return fmt.Errorf("unable to tag images: %s", err)
		}
		defer cancel()
		imageService := client.ImageService()
		image, err := imageService.Get(ctx, tags[0])
		if err != nil {
			return fmt.Errorf("unable to tag image: %s", err)
		}
		for _, targetRef := range tags[1:] {
			image.Name = targetRef
			if _, err := imageService.Create(ctx, image); err != nil {
				// if already exists; skip.
				if errors.Is(err, errdefs.ErrAlreadyExists) {
					continue
				}
				return fmt.Errorf("unable to tag image: %s", err)
			}
		}
	}

	return nil
}

func generateBuildctlArgs(cmd *cobra.Command, buildkitHost string, platform, args []string) (buildCtlBinary string,
	buildctlArgs []string, needsLoading bool, metaFile string, tags []string, cleanup func(), err error) {
	if len(args) < 1 {
		return "", nil, false, "", nil, nil, errors.New("context needs to be specified")
	}
	buildContext := args[0]
	if buildContext == "-" || strings.Contains(buildContext, "://") {
		return "", nil, false, "", nil, nil, fmt.Errorf("unsupported build context: %q", buildContext)
	}

	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}

	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}
	if output == "" {
		client, ctx, cancel, err := newClient(cmd)
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		defer cancel()
		info, err := client.Server(ctx)
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		ns, err := cmd.Flags().GetString("namespace")
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		snapshotter, err := cmd.Flags().GetString("snapshotter")
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		sharable, err := isImageSharable(buildkitHost, ns, info.UUID, snapshotter, platform)
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		if sharable {
			output = "type=image,unpack=true" // ensure the target stage is unlazied (needed for any snapshotters)
		} else {
			output = "type=docker"
			if len(platform) > 1 {
				// For avoiding `error: failed to solve: docker exporter does not currently support exporting manifest lists`
				// TODO: consider using type=oci for single-platform build too
				output = "type=oci"
			}
			needsLoading = true
		}
	} else {
		if !strings.Contains(output, "type=") {
			// should accept --output <DIR> as an alias of --output
			// type=local,dest=<DIR>
			output = fmt.Sprintf("type=local,dest=%s", output)
		}
		if strings.Contains(output, "type=docker") || strings.Contains(output, "type=oci") {
			needsLoading = true
		}
	}
	tagValue, err := cmd.Flags().GetStringArray("tag")
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}
	if tags = strutil.DedupeStrSlice(tagValue); len(tags) > 0 {
		ref := tags[0]
		named, err := dockerreference.ParseNormalizedNamed(ref)
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		output += ",name=" + dockerreference.TagNameOnly(named).String()

		// pick the first tag and add it to output
		for idx, tag := range tags {
			named, err := dockerreference.ParseNormalizedNamed(tag)
			if err != nil {
				return "", nil, false, "", nil, nil, err
			}
			tags[idx] = dockerreference.TagNameOnly(named).String()
		}
	} else if len(tags) == 0 {
		output = output + ",dangling-name-prefix=<none>"
	}

	buildctlArgs = buildkitutil.BuildctlBaseArgs(buildkitHost)

	progressValue, err := cmd.Flags().GetString("progress")
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}

	buildctlArgs = append(buildctlArgs, []string{
		"build",
		"--progress=" + progressValue,
		"--frontend=dockerfile.v0",
		"--local=context=" + buildContext,
		"--output=" + output,
	}...)

	filename, err := cmd.Flags().GetString("file")
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}

	dir := buildContext
	file := buildkitutil.DefaultDockerfileName
	if filename != "" {
		if filename == "-" {
			var err error
			dir, err = buildkitutil.WriteTempDockerfile(cmd.InOrStdin())
			if err != nil {
				return "", nil, false, "", nil, nil, err
			}
			cleanup = func() {
				os.RemoveAll(dir)
			}
		} else {
			dir, file = filepath.Split(filename)
		}

		if dir == "" {
			dir = "."
		}
	}
	dir, file, err = buildkitutil.BuildKitFile(dir, file)
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}

	buildctlArgs = append(buildctlArgs, "--local=dockerfile="+dir)
	buildctlArgs = append(buildctlArgs, "--opt=filename="+file)

	target, err := cmd.Flags().GetString("target")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	if target != "" {
		buildctlArgs = append(buildctlArgs, "--opt=target="+target)
	}

	if len(platform) > 0 {
		buildctlArgs = append(buildctlArgs, "--opt=platform="+strings.Join(platform, ","))
	}

	buildArgsValue, err := cmd.Flags().GetStringArray("build-arg")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	for _, ba := range strutil.DedupeStrSlice(buildArgsValue) {
		arr := strings.Split(ba, "=")
		if len(arr) == 1 && len(arr[0]) > 0 {
			// Avoid masking default build arg value from Dockerfile if environment variable is not set
			// https://github.com/moby/moby/issues/24101
			val, ok := os.LookupEnv(arr[0])
			if ok {
				buildctlArgs = append(buildctlArgs, fmt.Sprintf("--opt=build-arg:%s=%s", ba, val))
			} else {
				logrus.Debugf("ignoring unset build arg %q", ba)
			}
		} else if len(arr) > 1 && len(arr[0]) > 0 {
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
		} else {
			return "", nil, false, "", nil, nil, fmt.Errorf("invalid build arg %q", ba)
		}
	}

	labels, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}
	labels = strutil.DedupeStrSlice(labels)
	for _, l := range labels {
		buildctlArgs = append(buildctlArgs, "--opt=label:"+l)
	}

	noCache, err := cmd.Flags().GetBool("no-cache")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	if noCache {
		buildctlArgs = append(buildctlArgs, "--no-cache")
	}

	secretValue, err := cmd.Flags().GetStringArray("secret")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(secretValue) {
		buildctlArgs = append(buildctlArgs, "--secret="+s)
	}

	sshValue, err := cmd.Flags().GetStringArray("ssh")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(sshValue) {
		buildctlArgs = append(buildctlArgs, "--ssh="+s)
	}

	cacheFrom, err := cmd.Flags().GetStringArray("cache-from")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(cacheFrom) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--import-cache="+s)
	}

	cacheTo, err := cmd.Flags().GetStringArray("cache-to")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	for _, s := range strutil.DedupeStrSlice(cacheTo) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--export-cache="+s)
	}

	rm, err := cmd.Flags().GetBool("rm")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	if !rm {
		logrus.Warn("ignoring deprecated flag: '--rm=false'")
	}

	iidFile, err := cmd.Flags().GetString("iidfile")
	if err != nil {
		return "", nil, false, "", nil, cleanup, err
	}
	if iidFile != "" {
		file, err := os.CreateTemp("", "buildkit-meta-*")
		if err != nil {
			return "", nil, false, "", nil, cleanup, err
		}
		defer file.Close()
		metaFile = file.Name()
		buildctlArgs = append(buildctlArgs, "--metadata-file="+metaFile)
	}

	return buildctlBinary, buildctlArgs, needsLoading, metaFile, tags, cleanup, nil
}

func getDigestFromMetaFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	defer os.Remove(path)

	metadata := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		logrus.WithError(err).Errorf("failed to unmarshal metadata file %s", path)
		return "", err
	}
	digestRaw, ok := metadata["containerimage.digest"]
	if !ok {
		return "", errors.New("failed to find containerimage.digest in metadata file")
	}
	var digest string
	if err := json.Unmarshal(digestRaw, &digest); err != nil {
		logrus.WithError(err).Errorf("failed to unmarshal digset")
		return "", err
	}
	return digest, nil
}
