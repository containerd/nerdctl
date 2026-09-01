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

	"github.com/containerd/cgroups/v3/cgroup2"
)

// RootlessCgroup2GroupPath returns the cgroup v2 path of the RootlessKit child.
// The path includes any outer hierarchy imposed by the host, such as WSL's
// per-distribution cgroup prefix.
func RootlessCgroup2GroupPath() (string, error) {
	stateDir, err := RootlessKitStateDir()
	if err != nil {
		return "", err
	}
	return rootlessCgroup2GroupPath(stateDir, cgroup2.PidGroupPath)
}

func rootlessCgroup2GroupPath(stateDir string, pidGroupPath func(int) (string, error)) (string, error) {
	if pidGroupPath == nil {
		return "", errors.New("cgroup path resolver is not configured")
	}
	childPid, err := RootlessKitChildPid(stateDir)
	if err != nil {
		return "", fmt.Errorf("reading RootlessKit child PID: %w", err)
	}
	groupPath, err := pidGroupPath(childPid)
	if err != nil {
		return "", fmt.Errorf("reading cgroup v2 path for RootlessKit child PID %d: %w", childPid, err)
	}
	if err := cgroup2.VerifyGroupPath(groupPath); err != nil {
		return "", fmt.Errorf("invalid cgroup v2 path %q for RootlessKit child PID %d: %w", groupPath, childPid, err)
	}
	return groupPath, nil
}
