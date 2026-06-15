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

package manifest

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/cmd/manifest"
)

func removeCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "rm INDEX/MANIFESTLIST [INDEX/MANIFESTLIST...]",
		Short:             "Remove one or more index/manifest lists",
		Args:              cobra.MinimumNArgs(1),
		RunE:              removeAction,
		ValidArgsFunction: removeShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return cmd
}

func removeAction(cmd *cobra.Command, refs []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	var errs []error
	for _, ref := range refs {
		err := manifest.Remove(cmd.Context(), ref, globalOptions)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func removeShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
