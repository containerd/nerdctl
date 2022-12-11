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

package completion

import (
	"strings"

	"github.com/containerd/containerd/pkg/cap"
	"github.com/containerd/nerdctl/pkg/apparmorutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/spf13/cobra"
)

func ShellCompleteApparmorProfiles(cmd *cobra.Command) ([]string, cobra.ShellCompDirective) {
	profiles, err := apparmorutil.Profiles()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string // nolint: prealloc
	for _, f := range profiles {
		names = append(names, f.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func CapShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{}
	for _, c := range cap.Known() {
		// "CAP_SYS_ADMIN" -> "sys_admin"
		s := strings.ToLower(strings.TrimPrefix(c, "CAP_"))
		candidates = append(candidates, s)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func RunShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return ShellCompleteImageNames(cmd)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func ShellCompleteCgroupManagerNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{"cgroupfs"}
	if defaults.IsSystemdAvailable() {
		candidates = append(candidates, "systemd")
	}
	if rootlessutil.IsRootless() {
		candidates = append(candidates, "none")
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func ApparmorUnloadShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return ShellCompleteApparmorProfiles(cmd)
}
