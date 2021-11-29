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

package buildkitutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/containerd/nerdctl/pkg/rootlessutil"

	"github.com/sirupsen/logrus"
)

const (
	// DefaultDockerfileName is the Default filename, read by nerdctl build
	DefaultDockerfileName string = "Dockerfile"

	TempDockerfileName string = "docker-build-tempdockerfile-"
)

func BuildctlBinary() (string, error) {
	return exec.LookPath("buildctl")
}

func BuildctlBaseArgs(buildkitHost string) []string {
	return []string{"--addr=" + buildkitHost}
}

func PingBKDaemon(buildkitHost string) error {
	if runtime.GOOS != "linux" {
		return errors.New("only linux is supported")
	}
	hint := "`buildctl` needs to be installed and `buildkitd` needs to be running, see https://github.com/moby/buildkit"
	if rootlessutil.IsRootless() {
		hint += " , and `containerd-rootless-setuptool.sh install-buildkit`"
	}
	buildctlBinary, err := BuildctlBinary()
	if err != nil {
		return fmt.Errorf(hint+": %w", err)
	}
	args := BuildctlBaseArgs(buildkitHost)
	args = append(args, "debug", "workers")
	buildctlCheckCmd := exec.Command(buildctlBinary, args...)
	buildctlCheckCmd.Env = os.Environ()
	if out, err := buildctlCheckCmd.CombinedOutput(); err != nil {
		logrus.Error(string(out))
		return fmt.Errorf(hint+": %w", err)
	}
	return nil
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
