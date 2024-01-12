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

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/idutil/imagewalker"

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

var validInspectType = map[string]bool{
	"container": true,
	"image":     true,
}

func addInspectFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("type", "", "Return JSON for specified type")
	cmd.RegisterFlagCompletionFunc("type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"image", "container", ""}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	cmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func inspectAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	namespace := globalOptions.Namespace
	address := globalOptions.Address
	inspectType, err := cmd.Flags().GetString("type")
	if err != nil {
		return err
	}

	if len(inspectType) > 0 && !validInspectType[inspectType] {
		return fmt.Errorf("%q is not a valid value for --type", inspectType)
	}

	// container and image inspect can share the same client, since no `platform`
	// flag will be passed for image inspect.
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), namespace, address)
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

	inspectImage := len(inspectType) == 0 || inspectType == "image"
	inspectContainer := len(inspectType) == 0 || inspectType == "container"

	var imageInspectOptions types.ImageInspectOptions
	var containerInspectOptions types.ContainerInspectOptions
	if inspectImage {
		platform := ""
		imageInspectOptions, err = processImageInspectOptions(cmd, &platform)
		if err != nil {
			return err
		}
	}
	if inspectContainer {
		containerInspectOptions, err = processContainerInspectOptions(cmd)
		if err != nil {
			return err
		}
	}

	var errs []error
	for _, req := range args {
		var ni int
		var nc int

		if inspectImage {
			ni, err = imagewalker.Walk(ctx, req)
			if err != nil {
				return err
			}
		}
		if inspectContainer {
			nc, err = containerwalker.Walk(ctx, req)
			if err != nil {
				return err
			}
		}

		if ni == 0 && nc == 0 {
			errs = append(errs, fmt.Errorf("no such object %s", req))
		} else if ni > 0 {
			if err := image.Inspect(ctx, client, []string{req}, imageInspectOptions); err != nil {
				errs = append(errs, err)
			}
		} else if nc > 0 {
			if err := container.Inspect(ctx, client, []string{req}, containerInspectOptions); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d errors: %v", len(errs), errs)
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
