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
	ncdefaults "github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/spf13/cobra"
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
	// completion, login, logout: false, because it shouldn't require the daemon to be running
	// apparmor: false, because it requires the initial mount namespace to access /sys/kernel/security
	// cp, compose cp: false, because it requires the initial mount namespace to inspect file owners
	case "", "completion", "login", "logout", "apparmor", "cp":
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

func shellCompleteCgroupManagerNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{"cgroupfs"}
	if ncdefaults.IsSystemdAvailable() {
		candidates = append(candidates, "systemd")
	}
	if rootlessutil.IsRootless() {
		candidates = append(candidates, "none")
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func addApparmorCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newApparmorCommand())
}

func addCpCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newCpCommand())
}
