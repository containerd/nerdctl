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
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newSystemPruneCommand() *cobra.Command {
	systemPruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove unused data",
		Args:          cobra.NoArgs,
		RunE:          systemPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	systemPruneCommand.Flags().BoolP("all", "a", false, "Remove all unused images, not just dangling ones")
	systemPruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	systemPruneCommand.Flags().Bool("volumes", false, "Prune volumes")
	return systemPruneCommand
}

func systemPruneAction(cmd *cobra.Command, args []string) error {
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	if !all {
		logrus.Warn("Currently, `nerdctl system prune` requires --all to be specified. Skip pruning.")
		// NOP
		return nil
	}

	vFlag, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return err
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	if !force {
		var confirm string
		msg := `This will remove:
  - all stopped containers
  - all networks not used by at least one container`
		if vFlag {
			msg += `
  - all volumes not used by at least one container`
		}
		msg += `
  - all images without at least one container associated to them
  - all build cache
`
		msg += "\nAre you sure you want to continue? [y/N] "
		fmt.Fprintf(cmd.OutOrStdout(), "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	if err := containerPrune(cmd, client, ctx); err != nil {
		return err
	}
	if err := networkPrune(cmd, client, ctx); err != nil {
		return err
	}
	if vFlag {
		if err := volumePrune(cmd, client, ctx); err != nil {
			return err
		}
	}
	if err := imagePrune(cmd, client, ctx); err != nil {
		return err
	}

	var buf bytes.Buffer
	outWriter := bufio.NewWriter(&buf)
	copiedCmd := *cmd
	copiedCmd.SetOut(outWriter)
	if err := builderPruneAction(&copiedCmd, args); err != nil {
		return err
	}
	parsedOutput, err := buildkitutil.ParseBuildctlPruneTableOutput(buf.Bytes())
	if err != nil {
		return err
	}
	// pretty print the output of prune build cache
	out := cmd.OutOrStdout()
	for _, row := range parsedOutput.Rows {
		_, _ = fmt.Fprintf(out, "%s\n", row.ID)
	}
	j, _ := json.Marshal(parsedOutput)
	fmt.Fprintf(out, "%s\n", j)

	fmt.Fprintf(out, "%s", buf.Bytes())
	return err
}
