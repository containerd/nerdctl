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
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

// setupSociCommand creates and sets up a SOCI command with common configuration
func setupSociCommand(gOpts types.GlobalCommandOptions) (*exec.Cmd, error) {
	sociExecutable, err := exec.LookPath("soci")
	if err != nil {
		log.L.WithError(err).Error("soci executable not found in path $PATH")
		log.L.Info("you might consider installing soci from: https://github.com/awslabs/soci-snapshotter/blob/main/docs/install.md")
		return nil, err
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

	return sociCmd, nil
}

// CheckSociVersion checks if the SOCI binary version is at least the required version
func CheckSociVersion(requiredVersion string) error {
	sociExecutable, err := exec.LookPath("soci")
	if err != nil {
		log.L.WithError(err).Error("soci executable not found in path $PATH")
		log.L.Info("you might consider installing soci from: https://github.com/awslabs/soci-snapshotter/blob/main/docs/install.md")
		return err
	}

	cmd := exec.Command(sociExecutable, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get SOCI version: %w", err)
	}

	// Parse the version string
	versionStr := string(output)
	// Handle format like "soci version v0.10.0 8bbfe951bbb411798ee85dbd908544df4a1619a8.m"
	re := regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(versionStr)
	if len(matches) < 2 {
		return fmt.Errorf("failed to parse SOCI version from output: %s", versionStr)
	}

	// Extract version number
	installedVersionStr := matches[1]

	// Parse versions using semver package
	installedVersion, err := semver.NewVersion(installedVersionStr)
	if err != nil {
		return fmt.Errorf("failed to parse installed SOCI version: %w", err)
	}

	reqVersion, err := semver.NewVersion(requiredVersion)
	if err != nil {
		return fmt.Errorf("failed to parse required SOCI version: %w", err)
	}

	// Compare versions
	if installedVersion.LessThan(reqVersion) {
		return fmt.Errorf("SOCI version %s is lower than the required version %s for the convert operation", installedVersion.String(), reqVersion.String())
	}

	return nil
}

// ConvertSociIndexV2 converts an image to SOCI format and returns the converted image reference with digest
func ConvertSociIndexV2(ctx context.Context, client *client.Client, srcRef string, destRef string, gOpts types.GlobalCommandOptions, sOpts types.SociOptions) (string, error) {
	// Check if SOCI version is at least 0.10.0 which is required for the convert operation
	if err := CheckSociVersion("0.10.0"); err != nil {
		return "", err
	}

	sociCmd, err := setupSociCommand(gOpts)
	if err != nil {
		return "", err
	}

	sociCmd.Args = append(sociCmd.Args, "convert")

	if sOpts.AllPlatforms {
		sociCmd.Args = append(sociCmd.Args, "--all-platforms")
	} else if len(sOpts.Platforms) > 0 {
		// multiple values need to be passed as separate, repeating flags in soci as it uses urfave
		// https://github.com/urfave/cli/blob/main/docs/v2/examples/flags.md#multiple-values-per-single-flag
		for _, p := range sOpts.Platforms {
			sociCmd.Args = append(sociCmd.Args, "--platform", p)
		}
	}

	if sOpts.SpanSize != -1 {
		sociCmd.Args = append(sociCmd.Args, "--span-size", strconv.FormatInt(sOpts.SpanSize, 10))
	}

	if sOpts.MinLayerSize != -1 {
		sociCmd.Args = append(sociCmd.Args, "--min-layer-size", strconv.FormatInt(sOpts.MinLayerSize, 10))
	}

	sociCmd.Args = append(sociCmd.Args, srcRef, destRef)

	log.L.Infof("Converting image from %s to %s using SOCI format", srcRef, destRef)

	err = processSociIO(sociCmd)
	if err != nil {
		return "", err
	}
	err = sociCmd.Wait()
	if err != nil {
		return "", err
	}

	// Get the converted image's digest
	img, err := client.GetImage(ctx, destRef)
	if err != nil {
		return "", fmt.Errorf("failed to get converted image: %w", err)
	}

	// Return the full reference with digest
	return fmt.Sprintf("%s@%s", destRef, img.Target().Digest), nil
}

// CreateSociIndexV1 creates a SOCI index(`rawRef`)
func CreateSociIndexV1(rawRef string, gOpts types.GlobalCommandOptions, allPlatform bool, platforms []string, sOpts types.SociOptions) error {
	sociCmd, err := setupSociCommand(gOpts)
	if err != nil {
		return err
	}

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

	log.L.Debugf("running soci %v", sociCmd.Args)

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

	sociCmd, err := setupSociCommand(gOpts)
	if err != nil {
		return err
	}

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

	log.L.Debugf("running soci %v", sociCmd.Args)

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
