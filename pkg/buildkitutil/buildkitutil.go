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

/*
   Portions from https://github.com/docker/cli/blob/v20.10.9/cli/command/image/build/context.go
   Copyright (C) Docker authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/docker/cli/blob/v20.10.9/NOTICE
*/

package buildkitutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultDockerfileName is the Default filename, read by nerdctl build
	DefaultDockerfileName string = "Dockerfile"
	ContainerfileName     string = "Containerfile"

	TempDockerfileName string = "docker-build-tempdockerfile-"
)

func BuildctlBinary() (string, error) {
	return exec.LookPath("buildctl")
}

func BuildctlBaseArgs(buildkitHost string) []string {
	return []string{"--addr=" + buildkitHost}
}

func GetBuildkitHost(namespace string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace must be specified")
	}
	// Try candidate locations of the current containerd namespace.
	run := "/run/"
	if rootlessutil.IsRootless() {
		var err error
		run, err = rootlessutil.XDGRuntimeDir()
		if err != nil {
			logrus.Warn(err)
			run = fmt.Sprintf("/run/user/%d", rootlessutil.ParentEUID())
		}
	}
	var hostRel []string
	if namespace != "default" {
		hostRel = append(hostRel, fmt.Sprintf("buildkit-%s/buildkitd.sock", namespace))
	}
	hostRel = append(hostRel, "buildkit-default/buildkitd.sock", "buildkit/buildkitd.sock")
	var allErr error
	for _, p := range hostRel {
		logrus.Debugf("Choosing the buildkit host %q, candidates=%v (in %q)", p, hostRel, run)
		buildkitHost := "unix://" + filepath.Join(run, p)
		_, err := pingBKDaemon(buildkitHost)
		if err == nil {
			logrus.Debugf("Chosen buildkit host %q", buildkitHost)
			return buildkitHost, nil
		}
		allErr = multierror.Append(allErr, fmt.Errorf("failed to ping to host %s: %w", buildkitHost, err))
	}
	logrus.WithError(allErr).Error(getHint())
	return "", fmt.Errorf("no buildkit host is available, tried %d candidates: %w", len(hostRel), allErr)
}

func GetWorkerLabels(buildkitHost string) (labels map[string]string, _ error) {
	buildctlBinary, err := BuildctlBinary()
	if err != nil {
		return nil, err
	}
	args := BuildctlBaseArgs(buildkitHost)
	args = append(args, "debug", "workers", "--format", "{{json .}}")
	buildctlCheckCmd := exec.Command(buildctlBinary, args...)
	buildctlCheckCmd.Env = os.Environ()
	out, err := buildctlCheckCmd.Output()
	if err != nil {
		return nil, err
	}
	var workers []json.RawMessage
	if err := json.Unmarshal(out, &workers); err != nil {
		return nil, err
	}
	if len(workers) == 0 {
		return nil, fmt.Errorf("no worker available")
	}
	metadata := map[string]json.RawMessage{}
	if err := json.Unmarshal(workers[0], &metadata); err != nil {
		return nil, err
	}
	labelsRaw, ok := metadata["labels"]
	if !ok {
		return nil, fmt.Errorf("worker doesn't have labels")
	}
	labels = map[string]string{}
	if err := json.Unmarshal(labelsRaw, &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

func getHint() string {
	hint := "`buildctl` needs to be installed and `buildkitd` needs to be running, see https://github.com/moby/buildkit"
	if rootlessutil.IsRootless() {
		hint += " , and `containerd-rootless-setuptool.sh install-buildkit` for OCI worker or `containerd-rootless-setuptool.sh install-buildkit-containerd` for containerd worker"
	}
	return hint
}

func PingBKDaemon(buildkitHost string) error {
	if out, err := pingBKDaemon(buildkitHost); err != nil {
		if out != "" {
			logrus.Error(out)
		}
		return fmt.Errorf(getHint()+": %w", err)
	}
	return nil
}

func pingBKDaemon(buildkitHost string) (output string, _ error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("only linux is supported")
	}
	buildctlBinary, err := BuildctlBinary()
	if err != nil {
		return "", err
	}
	args := BuildctlBaseArgs(buildkitHost)
	args = append(args, "debug", "workers")
	buildctlCheckCmd := exec.Command(buildctlBinary, args...)
	buildctlCheckCmd.Env = os.Environ()
	if out, err := buildctlCheckCmd.CombinedOutput(); err != nil {
		return string(out), err
	}
	return "", nil
}

// WriteTempDockerfile is from https://github.com/docker/cli/blob/v20.10.9/cli/command/image/build/context.go#L118
func WriteTempDockerfile(rc io.Reader) (dockerfileDir string, err error) {
	// err is a named return value, due to the defer call below.
	dockerfileDir, err = os.MkdirTemp("", TempDockerfileName)
	if err != nil {
		return "", fmt.Errorf("unable to create temporary context directory: %v", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dockerfileDir)
		}
	}()

	f, err := os.Create(filepath.Join(dockerfileDir, DefaultDockerfileName))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, rc); err != nil {
		return "", err
	}
	return dockerfileDir, nil
}

// Buildkit file returns the values for the following buildctl args
// --localfilename=dockerfile={absDir}
// --opt=filename={file}
func BuildKitFile(dir, inputfile string) (absDir string, file string, err error) {
	file = inputfile
	if file == "" || file == "." {
		file = DefaultDockerfileName
	}
	absDir, err = filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	if file != DefaultDockerfileName {
		if _, err := os.Lstat(filepath.Join(absDir, file)); err != nil {
			return "", "", err
		}
	} else {
		_, dErr := os.Lstat(filepath.Join(absDir, file))
		_, cErr := os.Lstat(filepath.Join(absDir, ContainerfileName))
		if dErr == nil && cErr == nil {
			// both files exist, prefer Dockerfile.
			dockerfile, err := os.ReadFile(filepath.Join(absDir, DefaultDockerfileName))
			if err != nil {
				return "", "", err
			}
			containerfile, err := os.ReadFile(filepath.Join(absDir, ContainerfileName))
			if err != nil {
				return "", "", err
			}
			if !bytes.Equal(dockerfile, containerfile) {
				logrus.Warnf("%s and %s have different contents, building with %s", DefaultDockerfileName, ContainerfileName, DefaultDockerfileName)
			}
		}
		if dErr != nil {
			if errors.Is(dErr, fs.ErrNotExist) {
				file = ContainerfileName
			} else {
				return "", "", dErr
			}
			if cErr != nil {
				return "", "", cErr
			}
		}
	}
	return absDir, file, nil
}
