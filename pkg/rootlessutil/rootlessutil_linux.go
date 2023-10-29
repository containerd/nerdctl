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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rootless-containers/rootlesskit/pkg/api/client"
)

func IsRootless() bool {
	return IsRootlessParent() || IsRootlessChild()
}

func ParentEUID() int {
	if !IsRootlessChild() {
		return os.Geteuid()
	}
	env := os.Getenv("ROOTLESSKIT_PARENT_EUID")
	if env == "" {
		panic("environment variable ROOTLESSKIT_PARENT_EUID is not set")
	}
	i, err := strconv.Atoi(env)
	if err != nil {
		panic(fmt.Errorf("failed to parse ROOTLESSKIT_PARENT_EUID=%q: %w", env, err))
	}
	return i
}

func ParentEGID() int {
	if !IsRootlessChild() {
		return os.Getegid()
	}
	env := os.Getenv("ROOTLESSKIT_PARENT_EGID")
	if env == "" {
		panic("environment variable ROOTLESSKIT_PARENT_EGID is not set")
	}
	i, err := strconv.Atoi(env)
	if err != nil {
		panic(fmt.Errorf("failed to parse ROOTLESSKIT_PARENT_EGID=%q: %w", env, err))
	}
	return i
}

func NewRootlessKitClient() (client.Client, error) {
	stateDir, err := RootlessKitStateDir()
	if err != nil {
		return nil, err
	}
	apiSock := filepath.Join(stateDir, "api.sock")
	return client.New(apiSock)
}

func DetectRootlesskitFeature(feature string) (bool, error) {
	rootlesskit := "rootlesskit"
	rootlesskitBinary, err := exec.LookPath(rootlesskit)
	if err != nil {
		return false, fmt.Errorf("%s binary is not installed: %w", rootlesskit, err)
	}
	cmd := exec.Command(rootlesskitBinary, "--help")
	cmd.Env = os.Environ()
	b, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("command \"%s --help\" failed, --help is not supported: %w", rootlesskitBinary, err)
	}
	if !strings.Contains(string(b), feature) {
		return false, nil
	}
	return true, nil
}

// RootlessContainredSockAddress returns sock address of rootless containerd based on https://github.com/containerd/nerdctl/blob/main/docs/faq.md#containerd-socket-address
func RootlessContainredSockAddress() (string, error) {
	stateDir, err := RootlessKitStateDir()
	if err != nil {
		return "", err
	}
	childPid, err := RootlessKitChildPid(stateDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(fmt.Sprintf("/proc/%d/root/run/containerd/containerd.sock", childPid)), nil
}
