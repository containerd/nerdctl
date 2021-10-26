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

package rootlessutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
)

func IsRootlessParent() bool {
	return os.Geteuid() != 0
}

func RootlessKitStateDir() (string, error) {
	if v := os.Getenv("ROOTLESSKIT_STATE_DIR"); v != "" {
		return v, nil
	}
	xdr, err := XDGRuntimeDir()
	if err != nil {
		return "", err
	}
	// "${XDG_RUNTIME_DIR}/containerd-rootless" is hardcoded in containerd-rootless.sh
	stateDir := filepath.Join(xdr, "containerd-rootless")
	if _, err := os.Stat(stateDir); err != nil {
		return "", err
	}
	return stateDir, nil
}

func RootlessKitChildPid(stateDir string) (int, error) {
	pidFilePath := filepath.Join(stateDir, "child_pid")
	if _, err := os.Stat(pidFilePath); err != nil {
		return 0, err
	}

	pidFileBytes, err := os.ReadFile(pidFilePath)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(pidFileBytes))
	return strconv.Atoi(pidStr)
}

func ParentMain() error {
	if !IsRootlessParent() {
		return errors.New("should not be called when !IsRootlessParent()")
	}
	stateDir, err := RootlessKitStateDir()
	logrus.Debugf("stateDir: %s", stateDir)
	if err != nil {
		return fmt.Errorf("rootless containerd not running? (hint: use `containerd-rootless-setuptool.sh install` to start rootless containerd): %w", err)
	}
	childPid, err := RootlessKitChildPid(stateDir)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// FIXME: remove dependency on `nsenter` binary
	arg0, err := exec.LookPath("nsenter")
	if err != nil {
		return err
	}
	// args are compatible with both util-linux nsenter and busybox nsenter
	args := []string{
		"-r/",     // root dir (busybox nsenter wants this to be explicitly specified),
		"-w" + wd, // work dir
		"--preserve-credentials",
		"-m", "-n", "-U",
		"-t", strconv.Itoa(childPid),
		"-F", // no fork
	}
	args = append(args, os.Args...)
	logrus.Debugf("rootless parent main: executing %q with %v", arg0, args)

	// Env vars corresponds to RootlessKit spec:
	// https://github.com/rootless-containers/rootlesskit/tree/v0.13.1#environment-variables
	os.Setenv("ROOTLESSKIT_STATE_DIR", stateDir)
	os.Setenv("ROOTLESSKIT_PARENT_EUID", strconv.Itoa(os.Geteuid()))
	os.Setenv("ROOTLESSKIT_PARENT_EGID", strconv.Itoa(os.Getegid()))
	return syscall.Exec(arg0, args, os.Environ())
}
