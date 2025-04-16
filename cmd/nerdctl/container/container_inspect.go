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

package container

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
)

func inspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "inspect [flags] CONTAINER [CONTAINER, ...]",
		Short:             "Display detailed information on one or more containers.",
		Long:              "Hint: set `--mode=native` for showing the full output",
		Args:              cobra.MinimumNArgs(1),
		RunE:              inspectAction,
		ValidArgsFunction: containerInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	cmd.Flags().BoolP("size", "s", false, "Display total file sizes")

	cmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

var validModeType = map[string]bool{
	"native":       true,
	"dockercompat": true,
}

func InspectOptions(cmd *cobra.Command) (opt types.ContainerInspectOptions, err error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return
	}
	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return
	}
	if len(mode) > 0 && !validModeType[mode] {
		err = fmt.Errorf("%q is not a valid value for --mode", mode)
		return
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return
	}

	size, err := cmd.Flags().GetBool("size")
	if err != nil {
		return
	}

	return types.ContainerInspectOptions{
		GOptions: globalOptions,
		Format:   format,
		Mode:     mode,
		Size:     size,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func inspectAction(cmd *cobra.Command, args []string) error {
	opt, err := InspectOptions(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), opt.GOptions.Namespace, opt.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	entries, err := container.Inspect(ctx, client, args, opt)
	if err != nil {
		return err
	}

	// Display
	if len(entries) > 0 {
		if formatErr := formatter.FormatSlice(opt.Format, opt.Stdout, entries); formatErr != nil {
			log.G(ctx).Error(formatErr)
		}
	}
	return err
}

func containerInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return completion.ContainerNames(cmd, nil)
}
