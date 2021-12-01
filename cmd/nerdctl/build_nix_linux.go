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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/continuity/fs"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/nixutil"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newBuildNixCommand() *cobra.Command {
	shortHelp := "Reproducible image building with Nix derivations (EXPERIMENTAL)"
	longHelp := shortHelp + `
Examples: https://github.com/containerd/nerdctl/tree/master/examples/nix-build

WARNING: This command is experimental and its behavior is subject to change.
`
	cmd := &cobra.Command{
		Use:           "build-nix",
		Short:         shortHelp,
		Long:          longHelp,
		RunE:          buildNixAction,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	cmd.Flags().String("nix-image", defaults.NixImage, "Nix image")
	cmd.Flags().StringP("file", "f", "default.nix", "Name of the Nix derivation")
	cmd.Flags().String("iidfile", "", "Write the image ID to the file")
	return cmd
}

func buildNixAction(cmd *cobra.Command, args []string) error {
	logrus.Warn("This command is experimental and subject to change.")

	nixImage, err := cmd.Flags().GetString("nix-image")
	if err != nil {
		return err
	}

	file, err := cmd.Flags().GetString("file")
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err != nil {
		return err
	}
	dir := filepath.Dir(file)
	dir, err = filepath.Abs(dir)
	if err != nil {
		return err
	}

	iidfile, err := cmd.Flags().GetString("iidfile")
	if err != nil {
		return err
	}

	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return err
	}
	if _, err := volStore.Get(nixutil.CacheVolumeName); err != nil {
		if !errors.Is(err, os.ErrNotExist) && !errdefs.IsNotFound(err) {
			return err
		}
		logrus.Debugf("Creating the cache volume %q", nixutil.CacheVolumeName)
		if _, err := volStore.Create(nixutil.CacheVolumeName, nil); err != nil {
			return err
		}
	} else {
		logrus.Debugf("Using the existing cache volume %q", nixutil.CacheVolumeName)
	}
	tmpVolName := idgen.GenerateID()
	logrus.Debugf("Creating a tmp volume %q for storing the Nix derivation file directory %q", tmpVolName, dir)
	tmpVol, err := volStore.Create(tmpVolName, nil)
	if err != nil {
		return err
	}
	defer func() {
		logrus.Debugf("Removing tmp volume %q", tmpVolName)
		if _, rmVolErr := volStore.Remove([]string{tmpVolName}); rmVolErr != nil {
			logrus.WithError(rmVolErr).Warnf("Failed to remove volume %q", tmpVolName)
		}
	}()
	if err := fs.CopyDir(tmpVol.Mountpoint, dir); err != nil {
		return err
	}

	logrus.WithField("image", nixImage).Infof("Creating a Nix container")
	nerdctlCmd, nerdctlArgs := globalFlags(cmd)
	runCmd := exec.Command(nerdctlCmd, append(nerdctlArgs, []string{
		"run",
		"-d",
		"-v", tmpVolName + ":/mnt",
		"-v", nixutil.CacheVolumeName + ":/nix",
		"-w", "/mnt",
		nixImage,
		"sleep", "infinity",
	}...)...)
	var runStdout bytes.Buffer
	runCmd.Stdout = &runStdout
	runCmd.Stderr = cmd.ErrOrStderr()
	logrus.Debugf("Running %v", runCmd.Args)
	if err := runCmd.Run(); err != nil {
		return fmt.Errorf("failed to start a Nix container: %v: stdout=%q: %w", runCmd.Args, runStdout.String(), err)
	}
	containerID := strings.TrimSpace(runStdout.String())
	defer func() {
		rmCmd := exec.Command(nerdctlCmd, append(nerdctlArgs, []string{"rm", "-f", containerID}...)...)
		logrus.Debugf("Running %v", rmCmd.Args)
		if out, rmErr := rmCmd.CombinedOutput(); err != nil {
			logrus.WithError(rmErr).Warnf("failed to remove a Nix container %q: %v: %q", containerID, rmCmd.Args, string(out))
		}
	}()

	logrus.Info("Starting Nix")
	execCmd := exec.Command(nerdctlCmd, append(nerdctlArgs, []string{"exec", containerID, "nix-build", "--option", "build-users-group", "", filepath.Base(file)}...)...)
	execCmd.Stderr = cmd.ErrOrStderr()
	execCmd.Stdout = execCmd.Stderr
	logrus.Debugf("Running %v", execCmd.Args)
	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to run %v: %w", execCmd, err)
	}

	gunzipResultCmd := exec.Command(nerdctlCmd, append(nerdctlArgs, []string{"exec", containerID, "gzip", "-dc", "result"}...)...)
	gunzipResultCmd.Stderr = cmd.ErrOrStderr()
	catResultStdout, err := gunzipResultCmd.StdoutPipe()
	if err != nil {
		return err
	}

	logrus.Debugf("Running %v", gunzipResultCmd.Args)
	if err = gunzipResultCmd.Start(); err != nil {
		return err
	}
	dockerArchiveDigester := digest.SHA256.Digester()
	dockerArchiveHasher := dockerArchiveDigester.Hash()
	teeReader := io.TeeReader(catResultStdout, dockerArchiveHasher)
	loaded, err := loadImage(teeReader, cmd, args, nil, false)
	if err != nil {
		return err
	}
	if err := gunzipResultCmd.Wait(); err != nil {
		return err
	}
	dockerArchiveDigest := dockerArchiveDigester.Digest()
	logrus.Infof("Tar digest: %s", dockerArchiveDigest)
	if len(loaded) == 0 {
		return errors.New("no image was loaded")
	}
	if len(loaded) > 1 {
		logrus.Warnf("Unexpectedly loaded multiple images: %v", loaded)
	}
	imageID := loaded[0].Target.Digest
	logrus.Infof("Image ID: %s", imageID)
	if iidfile != "" {
		if err := os.WriteFile(iidfile, []byte(imageID), 0644); err != nil {
			return err
		}
	}
	return nil
}
