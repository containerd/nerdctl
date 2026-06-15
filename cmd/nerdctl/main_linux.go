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
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/apparmor"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

func appNeedsRootlessParentMain(cmd *cobra.Command, args []string) bool {
	commands := []string{}
	for tcmd := cmd; tcmd != nil; tcmd = tcmd.Parent() {
		commands = append(commands, tcmd.Name())
	}
	commands = strutil.ReverseStrSlice(commands)

	if !rootlessutil.IsRootlessParent() {
		return false
	}
	if len(commands) < 2 {
		return true
	}
	switch commands[1] {
	// completion, login, logout, version: false, because it shouldn't require the daemon to be running
	// apparmor: false, because it requires the initial mount namespace to access /sys/kernel/security
	// cp, compose cp: false, because it requires the initial mount namespace to inspect file owners
	case "", "completion", "login", "logout", "apparmor", "cp", "version":
		return false
	case "container":
		if len(commands) < 3 {
			return true
		}
		switch commands[2] {
		case "cp":
			return false
		}
	case "compose":
		if len(commands) < 3 {
			return true
		}
		switch commands[2] {
		case "cp":
			return false
		}
	}
	return true
}

func addApparmorCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(apparmor.Command())
}

// resetSavedSETUID drops the saved UID of a setuid-root process to the original real UID.
// This ensures the process cannot regain root privileges later.
// It only performs the operation if the process is currently running with effective UID 0 (root)
// and was started by a non-root user (i.e., real UID != effective UID).
// For more info see issue https://github.com/containerd/nerdctl/issues/4098
func resetSavedSETUID() error {
	var err error
	uid := unix.Getuid()
	euid := unix.Geteuid()
	if uid != euid && euid == 0 {
		err = unix.Setresuid(0, 0, uid)
	}
	return err
}
