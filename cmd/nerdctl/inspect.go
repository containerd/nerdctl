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

	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"

	"github.com/spf13/cobra"
)

func newInspectCommand() *cobra.Command {
	var inspectCommand = &cobra.Command{
		Use:               "inspect",
		Short:             "Return low-level information on objects.",
		Args:              cobra.MinimumNArgs(1),
		RunE:              inspectAction,
		ValidArgsFunction: inspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	addInspectFlags(inspectCommand)

	return inspectCommand
}

func addInspectFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	cmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func inspectAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	imagewalker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			return nil
		},
	}

	containerwalker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			return nil
		},
	}

	for _, req := range args {
		ni, err := imagewalker.Walk(ctx, req)
		if err != nil {
			return err
		}

		nc, err := containerwalker.Walk(ctx, req)
		if err != nil {
			return err
		}

		if ni != 0 && nc != 0 || nc > 1 {
			return fmt.Errorf("multiple IDs found with provided prefix: %s", req)
		}

		if nc == 0 && ni == 0 {
			return fmt.Errorf("no such object %s", req)
		}
		if ni != 0 {
			if err := imageInspectActionWithPlatform(cmd, args, ""); err != nil {
				return err
			}
		} else {
			if err := containerInspectAction(cmd, args); err != nil {
				return err
			}
		}
	}

	return nil
}

func inspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	containers, _ := shellCompleteContainerNames(cmd, nil)
	// show image names
	images, _ := shellCompleteImageNames(cmd)
	return append(containers, images...), cobra.ShellCompDirectiveNoFileComp
}
