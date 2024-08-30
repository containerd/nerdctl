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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/apparmor"
)

func newApparmorLsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List the loaded AppArmor profiles",
		Args:          cobra.NoArgs,
		RunE:          apparmorLsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("quiet", "q", false, "Only display profile names")
	// Alias "-f" is reserved for "--filter"
	cmd.Flags().String("format", "", "Format the output using the given go template")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func processApparmorListOptions(cmd *cobra.Command) (types.ApparmorListOptions, error) {
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return types.ApparmorListOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.ApparmorListOptions{}, err
	}
	return types.ApparmorListOptions{
		Quiet:  quiet,
		Format: format,
		Stdout: cmd.OutOrStdout(),
	}, nil
}

func apparmorLsAction(cmd *cobra.Command, args []string) error {
	options, err := processApparmorListOptions(cmd)
	if err != nil {
		return err
	}
	return apparmor.List(options)
}
