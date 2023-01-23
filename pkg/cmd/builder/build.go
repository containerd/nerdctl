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

package builder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/platforms"
	dockerreference "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/sirupsen/logrus"
)

func Build(ctx context.Context, options types.BuilderBuildOptions) error {
	buildctlBinary, buildctlArgs, needsLoading, metaFile, tags, cleanup, err := generateBuildctlArgs(ctx, options)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
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
		buildctlCmd.Stdout = options.Stdout
	}
	if !options.Quiet {
		buildctlCmd.Stderr = options.Stderr
	}

	if err := buildctlCmd.Start(); err != nil {
		return err
	}

	if needsLoading {
		platMC, err := platformutil.NewMatchComparer(false, options.Platform)
		if err != nil {
			return err
		}
		if err = loadImage(ctx, buildctlStdout, options.GOptions.Namespace, options.GOptions.Address, options.GOptions.Snapshotter, options.Stdout, platMC, options.Quiet); err != nil {
			return err
		}
	}

	if err = buildctlCmd.Wait(); err != nil {
		return err
	}

	if options.IidFile != "" {
		id, err := getDigestFromMetaFile(metaFile)
		if err != nil {
			return err
		}
		if err := os.WriteFile(options.IidFile, []byte(id), 0600); err != nil {
			return err
		}
	}

	if len(tags) > 1 {
		client, ctx, cancel, err := clientutil.NewClient(ctx, options.GOptions.Namespace, options.GOptions.Address)
		logrus.Debug("Found more than 1 tag")
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

// TODO: This struct and `loadImage` are duplicated with the code in `cmd/load.go`, remove it after `load.go` has been refactor
type readCounter struct {
	io.Reader
	N int
}

func loadImage(ctx context.Context, in io.Reader, namespace, address, snapshotter string, output io.Writer, platMC platforms.MatchComparer, quiet bool) error {
	// In addition to passing WithImagePlatform() to client.Import(), we also need to pass WithDefaultPlatform() to NewClient().
	// Otherwise unpacking may fail.
	client, ctx, cancel, err := clientutil.NewClient(ctx, namespace, address, containerd.WithDefaultPlatform(platMC))
	if err != nil {
		return err
	}
	defer cancel()

	r := &readCounter{Reader: in}
	imgs, err := client.Import(ctx, r, containerd.WithDigestRef(archive.DigestTranslator(snapshotter)), containerd.WithSkipDigestRef(func(name string) bool { return name != "" }), containerd.WithImportPlatform(platMC))
	if err != nil {
		if r.N == 0 {
			// Avoid confusing "unrecognized image format"
			return errors.New("no image was built")
		}
		if errors.Is(err, images.ErrEmptyWalk) {
			err = fmt.Errorf("%w (Hint: set `--platform=PLATFORM` or `--all-platforms`)", err)
		}
		return err
	}
	for _, img := range imgs {
		image := containerd.NewImageWithPlatform(client, img, platMC)

		// TODO: Show unpack status
		if !quiet {
			fmt.Fprintf(output, "unpacking %s (%s)...\n", img.Name, img.Target.Digest)
		}
		err = image.Unpack(ctx, snapshotter)
		if err != nil {
			return err
		}
		if quiet {
			fmt.Fprintln(output, img.Target.Digest)
		} else {
			fmt.Fprintf(output, "Loaded image: %s\n", img.Name)
		}
	}

	return nil
}

func generateBuildctlArgs(ctx context.Context, options types.BuilderBuildOptions) (buildCtlBinary string,
	buildctlArgs []string, needsLoading bool, metaFile string, tags []string, cleanup func(), err error) {

	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		return "", nil, false, "", nil, nil, err
	}

	output := options.Output
	if output == "" {
		client, ctx, cancel, err := clientutil.NewClient(ctx, options.GOptions.Namespace, options.GOptions.Address)
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		defer cancel()
		info, err := client.Server(ctx)
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		sharable, err := isImageSharable(options.BuildKitHost, options.GOptions.Namespace, info.UUID, options.GOptions.Snapshotter, options.Platform)
		if err != nil {
			return "", nil, false, "", nil, nil, err
		}
		if sharable {
			output = "type=image,unpack=true" // ensure the target stage is unlazied (needed for any snapshotters)
		} else {
			output = "type=docker"
			if len(options.Platform) > 1 {
				// For avoiding `error: failed to solve: docker exporter does not currently support exporting manifest lists`
				// TODO: consider using type=oci for single-options.Platform build too
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
	if tags = strutil.DedupeStrSlice(options.Tag); len(tags) > 0 {
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

	buildctlArgs = buildkitutil.BuildctlBaseArgs(options.BuildKitHost)

	buildctlArgs = append(buildctlArgs, []string{
		"build",
		"--progress=" + options.Progress,
		"--frontend=dockerfile.v0",
		"--local=context=" + options.BuildContext,
		"--output=" + output,
	}...)

	dir := options.BuildContext
	file := buildkitutil.DefaultDockerfileName
	if options.File != "" {
		if options.File == "-" {
			// Super Warning: this is a special trick to update the dir variable, Don't move this line!!!!!!
			var err error
			dir, err = buildkitutil.WriteTempDockerfile(options.Stdin)
			if err != nil {
				return "", nil, false, "", nil, nil, err
			}
			cleanup = func() {
				os.RemoveAll(dir)
			}
		} else {
			dir, file = filepath.Split(options.File)
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

	if options.Target != "" {
		buildctlArgs = append(buildctlArgs, "--opt=target="+options.Target)
	}

	if len(options.Platform) > 0 {
		buildctlArgs = append(buildctlArgs, "--opt=platform="+strings.Join(options.Platform, ","))
	}

	for _, ba := range strutil.DedupeStrSlice(options.BuildArgs) {
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

	for _, l := range strutil.DedupeStrSlice(options.Label) {
		buildctlArgs = append(buildctlArgs, "--opt=label:"+l)
	}

	if options.NoCache {
		buildctlArgs = append(buildctlArgs, "--no-cache")
	}

	for _, s := range strutil.DedupeStrSlice(options.Secret) {
		buildctlArgs = append(buildctlArgs, "--secret="+s)
	}

	for _, s := range strutil.DedupeStrSlice(options.SSH) {
		buildctlArgs = append(buildctlArgs, "--ssh="+s)
	}

	for _, s := range strutil.DedupeStrSlice(options.CacheFrom) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--import-cache="+s)
	}

	for _, s := range strutil.DedupeStrSlice(options.CacheTo) {
		if !strings.Contains(s, "type=") {
			s = "type=registry,ref=" + s
		}
		buildctlArgs = append(buildctlArgs, "--export-cache="+s)
	}

	if !options.Rm {
		logrus.Warn("ignoring deprecated flag: '--rm=false'")
	}

	if options.IidFile != "" {
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

func isImageSharable(buildkitHost, namespace, uuid, snapshotter string, platform []string) (bool, error) {
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
