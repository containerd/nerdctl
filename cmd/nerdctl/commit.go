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
	"context"
	"fmt"

	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/imgutil/commit"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func newCommitCommand() *cobra.Command {
	var commitCommand = &cobra.Command{
		Use:               "commit [flags] CONTAINER REPOSITORY[:TAG]",
		Short:             "Create a new image from a container's changes",
		RunE:              commitAction,
		ValidArgsFunction: commitShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	commitCommand.Flags().StringP("author", "a", "", `Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")`)
	commitCommand.Flags().StringP("message", "m", "", "Commit message")
	return commitCommand
}

func commitAction(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return errors.New("need container and commit image name")
	}

	opts, err := newCommitOpts(cmd, args)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return errors.Errorf("ambiguous ID %q", found.Req)
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
		return errors.Errorf("no such container %s", req)
	}
	return nil
}

func newCommitOpts(cmd *cobra.Command, args []string) (*commit.Opts, error) {
	rawRef := args[1]

	named, err := refdocker.ParseDockerRef(rawRef)
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

	return &commit.Opts{
		Author:  author,
		Message: message,
		Ref:     named.String(),
	}, nil
}

func commitShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return shellCompleteContainerNames(cmd, nil)
	} else {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
