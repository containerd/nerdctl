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

package buildkitutil

import (
	"os"
	"os/exec"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func BuildctlBinary() (string, error) {
	return exec.LookPath("buildctl")
}

func BuildctlBaseArgs(buildkitHost string) []string {
	return []string{"--addr=" + buildkitHost}
}

func PingBKDaemon(buildkitHost string) error {
	const hint = "`buildctl` needs to be installed and `buildkitd` needs to be running, see https://github.com/moby/buildkit"
	buildctlBinary, err := BuildctlBinary()
	if err != nil {
		return errors.Wrap(err, hint)
	}
	args := BuildctlBaseArgs(buildkitHost)
	args = append(args, "debug", "workers")
	buildctlCheckCmd := exec.Command(buildctlBinary, args...)
	buildctlCheckCmd.Env = os.Environ()
	if out, err := buildctlCheckCmd.CombinedOutput(); err != nil {
		logrus.Error(string(out))
		return errors.Wrap(err, hint)
	}
	return nil
}
