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
	"os"

	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

func newExportCommand() *cobra.Command {
	var exportCommand = &cobra.Command{
		Use:               "export CONTAINER",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Export a containers filesystem as a tar archive",
		Long:              "Export a containers filesystem as a tar archive",
		RunE:              exportAction,
		ValidArgsFunction: exportShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	exportCommand.Flags().StringP("output", "o", "", "Write to a file, instead of STDOUT")

	return exportCommand
}

func exportAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("requires at least 1 argument")
	}

	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	writer := cmd.OutOrStdout()
	if output != "" {
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		writer = f
	} else {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("cowardly refusing to save to a terminal. Use the -o flag or redirect")
		}
	}
	return container.Export(ctx, client, args, writer)

}

func exportShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return shellCompleteContainerNames(cmd, nil)
}
