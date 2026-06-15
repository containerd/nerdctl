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

func createCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "create [OPTIONS] CONTAINER CHECKPOINT",
		Short:             "Create a checkpoint from a running container",
		Args:              cobra.ExactArgs(2),
		RunE:              createAction,
		ValidArgsFunction: createShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().Bool("leave-running", false, "Leave the container running after checkpointing")
	cmd.Flags().String("checkpoint-dir", "", "Checkpoint directory")
	return cmd
}

func processCreateFlags(cmd *cobra.Command) (types.CheckpointCreateOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.CheckpointCreateOptions{}, err
	}

	leaveRunning, err := cmd.Flags().GetBool("leave-running")
	if err != nil {
		return types.CheckpointCreateOptions{}, err
	}
	checkpointDir, err := cmd.Flags().GetString("checkpoint-dir")
	if err != nil {
		return types.CheckpointCreateOptions{}, err
	}
	if checkpointDir == "" {
		checkpointDir = filepath.Join(globalOptions.DataRoot, "checkpoints")
	}

	return types.CheckpointCreateOptions{
		Stdout:        cmd.OutOrStdout(),
		GOptions:      globalOptions,
		LeaveRunning:  leaveRunning,
		CheckpointDir: checkpointDir,
	}, nil
}

func createAction(cmd *cobra.Command, args []string) error {
	createOptions, err := processCreateFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), createOptions.GOptions.Namespace, createOptions.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	err = checkpoint.Create(ctx, client, args[0], args[1], createOptions)
	if err != nil {
		return err
	}

	return nil
}

func createShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
