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
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/checkpoint"
)

func removeCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "rm [OPTIONS] CONTAINER CHECKPOINT",
		Short:             "Remove a checkpoint",
		Args:              cobra.ExactArgs(2),
		RunE:              removeAction,
		ValidArgsFunction: removeShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().String("checkpoint-dir", "", "Checkpoint directory")
	return cmd
}

func processRemoveFlags(cmd *cobra.Command) (types.CheckpointRemoveOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.CheckpointRemoveOptions{}, err
	}

	checkpointDir, err := cmd.Flags().GetString("checkpoint-dir")
	if err != nil {
		return types.CheckpointRemoveOptions{}, err
	}
	if checkpointDir == "" {
		checkpointDir = filepath.Join(globalOptions.DataRoot, "checkpoints")
	}

	return types.CheckpointRemoveOptions{
		Stdout:        cmd.OutOrStdout(),
		GOptions:      globalOptions,
		CheckpointDir: checkpointDir,
	}, nil
}

func removeAction(cmd *cobra.Command, args []string) error {
	removeOptions, err := processRemoveFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), removeOptions.GOptions.Namespace, removeOptions.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	err = checkpoint.Remove(ctx, client, args[0], args[1], removeOptions)
	if err != nil {
		return err
	}

	return nil
}

func removeShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
