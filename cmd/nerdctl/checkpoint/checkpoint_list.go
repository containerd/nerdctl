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

package checkpoint

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/checkpoint"
)

func listCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "list [OPTIONS] CONTAINER",
		Short:             "List checkpoints for a container",
		Args:              cobra.ExactArgs(1),
		RunE:              listAction,
		ValidArgsFunction: listShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().String("checkpoint-dir", "", "Checkpoint directory")
	return cmd
}

func processListFlags(cmd *cobra.Command) (types.CheckpointListOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.CheckpointListOptions{}, err
	}

	checkpointDir, err := cmd.Flags().GetString("checkpoint-dir")
	if err != nil {
		return types.CheckpointListOptions{}, err
	}
	if checkpointDir == "" {
		checkpointDir = globalOptions.DataRoot + "/checkpoints"
	}

	return types.CheckpointListOptions{
		Stdout:        cmd.OutOrStdout(),
		GOptions:      globalOptions,
		CheckpointDir: checkpointDir,
	}, nil
}

func listAction(cmd *cobra.Command, args []string) error {
	listOptions, err := processListFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), listOptions.GOptions.Namespace, listOptions.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	checkpoints, err := checkpoint.List(ctx, client, args[0], listOptions)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(listOptions.Stdout, 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "CHECKPOINT NAME")

	for _, cp := range checkpoints {
		fmt.Fprintln(w, cp.Name)
	}

	return w.Flush()
}

func listShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
