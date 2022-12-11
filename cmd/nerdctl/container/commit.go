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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/imgutil/commit"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewCommitCommand() *cobra.Command {
	var commitCommand = &cobra.Command{
		Use:               "commit [flags] CONTAINER REPOSITORY[:TAG]",
		Short:             "Create a new image from a container's changes",
		Args:              utils.IsExactArgs(2),
		RunE:              commitAction,
		ValidArgsFunction: commitShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	commitCommand.Flags().StringP("author", "a", "", `Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")`)
	commitCommand.Flags().StringP("message", "m", "", "Commit message")
	commitCommand.Flags().StringArrayP("change", "c", nil, "Apply Dockerfile instruction to the created image (supported directives: [CMD, ENTRYPOINT])")
	commitCommand.Flags().BoolP("pause", "p", true, "Pause container during commit")
	return commitCommand
}

func commitAction(cmd *cobra.Command, args []string) error {
	opts, err := newCommitOpts(cmd, args)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := nerdClient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			imageID, err := commit.Commit(ctx, client, found.Container, opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", imageID)
			return err
		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

func parseChanges(cmd *cobra.Command) (commit.Changes, error) {
	const (
		// XXX: Where can I get a constants for this?
		commandDirective    = "CMD"
		entrypointDirective = "ENTRYPOINT"
	)
	userChanges, err := cmd.Flags().GetStringArray("change")
	if err != nil || userChanges == nil {
		return commit.Changes{}, err
	}
	var changes commit.Changes
	for _, change := range userChanges {
		if change == "" {
			return commit.Changes{}, fmt.Errorf("received an empty value in change flag")
		}
		changeFields := strings.Fields(change)

		switch changeFields[0] {
		case commandDirective:
			var overrideCMD []string
			if err := json.Unmarshal([]byte(change[len(changeFields[0]):]), &overrideCMD); err != nil {
				return commit.Changes{}, fmt.Errorf("malformed json in change flag value %q", change)
			}
			if changes.CMD != nil {
				logrus.Warn("multiple change flags supplied for the CMD directive, overriding with last supplied")
			}
			changes.CMD = overrideCMD
		case entrypointDirective:
			var overrideEntrypoint []string
			if err := json.Unmarshal([]byte(change[len(changeFields[0]):]), &overrideEntrypoint); err != nil {
				return commit.Changes{}, fmt.Errorf("malformed json in change flag value %q", change)
			}
			if changes.Entrypoint != nil {
				logrus.Warnf("multiple change flags supplied for the Entrypoint directive, overriding with last supplied")
			}
			changes.Entrypoint = overrideEntrypoint
		default: // TODO: Support the rest of the change directives
			return commit.Changes{}, fmt.Errorf("unknown change directive %q", changeFields[0])
		}
	}
	return changes, nil
}

func newCommitOpts(cmd *cobra.Command, args []string) (*commit.Opts, error) {
	rawRef := args[1]

	named, err := referenceutil.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}

	author, err := cmd.Flags().GetString("author")
	if err != nil {
		return nil, err
	}
	message, err := cmd.Flags().GetString("message")
	if err != nil {
		return nil, err
	}
	pause, err := cmd.Flags().GetBool("pause")
	if err != nil {
		return nil, err
	}

	changes, err := parseChanges(cmd)
	if err != nil {
		return nil, err
	}

	return &commit.Opts{
		Author:  author,
		Message: message,
		Ref:     named.String(),
		Pause:   pause,
		Changes: changes,
	}, nil
}

func commitShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completion.ShellCompleteContainerNames(cmd, nil)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
