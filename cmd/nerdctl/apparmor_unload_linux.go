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
	"fmt"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/apparmor"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/spf13/cobra"
)

func newApparmorUnloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "unload [PROFILE]",
		Short:             fmt.Sprintf("Unload an AppArmor profile. The target profile name defaults to %q. Requires root.", defaults.AppArmorProfileName),
		Args:              cobra.MaximumNArgs(1),
		RunE:              apparmorUnloadAction,
		ValidArgsFunction: apparmorUnloadShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return cmd
}

func apparmorUnloadAction(cmd *cobra.Command, args []string) error {
	target := defaults.AppArmorProfileName
	if len(args) > 0 {
		target = args[0]
	}
	options := &types.ApparmorUnloadCommandOptions{}
	options.Target = target
	return apparmor.Unload(options)
}

func apparmorUnloadShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return shellCompleteApparmorProfiles(cmd)
}
