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

package snapshotterutil

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

// CreateSoci creates a SOCI index(`rawRef`)
func CreateSoci(rawRef string, gOpts types.GlobalCommandOptions, allPlatform bool, platforms []string, sOpts types.SociOptions) error {
	sociExecutable, err := exec.LookPath("soci")
	if err != nil {
		log.L.WithError(err).Error("soci executable not found in path $PATH")
		log.L.Info("you might consider installing soci from: https://github.com/awslabs/soci-snapshotter/blob/main/docs/install.md")
		return err
	}

	sociCmd := exec.Command(sociExecutable)
	sociCmd.Env = os.Environ()

	// #region for global flags.
	if gOpts.Address != "" {
		sociCmd.Args = append(sociCmd.Args, "--address", gOpts.Address)
	}
	if gOpts.Namespace != "" {
		sociCmd.Args = append(sociCmd.Args, "--namespace", gOpts.Namespace)
	}
	// #endregion

	// Global flags have to be put before subcommand before soci upgrades to urfave v3.
	// https://github.com/urfave/cli/issues/1113
	sociCmd.Args = append(sociCmd.Args, "create")

	if allPlatform {
		sociCmd.Args = append(sociCmd.Args, "--all-platforms")
	}
	if len(platforms) > 0 {
		// multiple values need to be passed as separate, repeating flags in soci as it uses urfave
		// https://github.com/urfave/cli/blob/main/docs/v2/examples/flags.md#multiple-values-per-single-flag
		for _, p := range platforms {
			sociCmd.Args = append(sociCmd.Args, "--platform", p)
		}
	}

	if sOpts.SpanSize != -1 {
		sociCmd.Args = append(sociCmd.Args, "--span-size", strconv.FormatInt(sOpts.SpanSize, 10))
	}
	if sOpts.MinLayerSize != -1 {
		sociCmd.Args = append(sociCmd.Args, "--min-layer-size", strconv.FormatInt(sOpts.MinLayerSize, 10))
	}
	// --timeout, --debug, --content-store
	sociCmd.Args = append(sociCmd.Args, rawRef)

	log.L.Debugf("running %s %v", sociExecutable, sociCmd.Args)

	err = processSociIO(sociCmd)
	if err != nil {
		return err
	}

	return sociCmd.Wait()
}

// PushSoci pushes a SOCI index(`rawRef`)
// `hostsDirs` are used to resolve image `rawRef`
func PushSoci(rawRef string, gOpts types.GlobalCommandOptions, allPlatform bool, platforms []string) error {
	log.L.Debugf("pushing SOCI index: %s", rawRef)

	sociExecutable, err := exec.LookPath("soci")
	if err != nil {
		log.L.WithError(err).Error("soci executable not found in path $PATH")
		log.L.Info("you might consider installing soci from: https://github.com/awslabs/soci-snapshotter/blob/main/docs/install.md")
		return err
	}

	sociCmd := exec.Command(sociExecutable)
	sociCmd.Env = os.Environ()

	// #region for global flags.
	if gOpts.Address != "" {
		sociCmd.Args = append(sociCmd.Args, "--address", gOpts.Address)
	}
	if gOpts.Namespace != "" {
		sociCmd.Args = append(sociCmd.Args, "--namespace", gOpts.Namespace)
	}
	// #endregion

	// Global flags have to be put before subcommand before soci upgrades to urfave v3.
	// https://github.com/urfave/cli/issues/1113
	sociCmd.Args = append(sociCmd.Args, "push")

	if allPlatform {
		sociCmd.Args = append(sociCmd.Args, "--all-platforms")
	}
	if len(platforms) > 0 {
		// multiple values need to be passed as separate, repeating flags in soci as it uses urfave
		// https://github.com/urfave/cli/blob/main/docs/v2/examples/flags.md#multiple-values-per-single-flag
		for _, p := range platforms {
			sociCmd.Args = append(sociCmd.Args, "--platform", p)
		}
	}
	if gOpts.InsecureRegistry {
		sociCmd.Args = append(sociCmd.Args, "--skip-verify")
		sociCmd.Args = append(sociCmd.Args, "--plain-http")
	}
	if len(gOpts.HostsDir) > 0 {
		sociCmd.Args = append(sociCmd.Args, "--hosts-dir")
		sociCmd.Args = append(sociCmd.Args, strings.Join(gOpts.HostsDir, ","))
	}
	sociCmd.Args = append(sociCmd.Args, rawRef)

	log.L.Debugf("running %s %v", sociExecutable, sociCmd.Args)

	err = processSociIO(sociCmd)
	if err != nil {
		return err
	}
	return sociCmd.Wait()
}

func processSociIO(sociCmd *exec.Cmd) error {
	stdout, err := sociCmd.StdoutPipe()
	if err != nil {
		log.L.Warn("soci: " + err.Error())
	}
	stderr, err := sociCmd.StderrPipe()
	if err != nil {
		log.L.Warn("soci: " + err.Error())
	}
	if err := sociCmd.Start(); err != nil {
		// only return err if it's critical (soci command failed to start.)
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		log.L.Info("soci: " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.L.Warn("soci: " + err.Error())
	}

	errScanner := bufio.NewScanner(stderr)
	for errScanner.Scan() {
		log.L.Info("soci: " + errScanner.Text())
	}
	if err := errScanner.Err(); err != nil {
		log.L.Warn("soci: " + err.Error())
	}

	return nil
}
