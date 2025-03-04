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

package apparmor

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/pkg/cmd/apparmor"
	"github.com/containerd/nerdctl/v2/pkg/defaults"
)

func unloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "unload [PROFILE]",
		Short:             fmt.Sprintf("Unload an AppArmor profile. The target profile name defaults to %q. Requires root.", defaults.AppArmorProfileName),
		Args:              cobra.MaximumNArgs(1),
		RunE:              unloadAction,
		ValidArgsFunction: unloadShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return cmd
}

func unloadAction(cmd *cobra.Command, args []string) error {
	target := defaults.AppArmorProfileName
	if len(args) > 0 {
		target = args[0]
	}
	return apparmor.Unload(target)
}

func unloadShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ApparmorProfiles(cmd)
}
