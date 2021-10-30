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
	"errors"
	"os"

	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/platformutil"

	"github.com/spf13/cobra"
)

func newPullCommand() *cobra.Command {
	var pullCommand = &cobra.Command{
		Use:           "pull",
		Short:         "Pull an image from a registry",
		RunE:          pullAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	pullCommand.PersistentFlags().StringSlice("platform", nil, "Pull content for a specific platform")
	pullCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	pullCommand.PersistentFlags().Bool("all-platforms", false, "Pull content for all platforms")
	// #endregion

	return pullCommand
}

func pullAction(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New("image name needs to be specified")
	}
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	insecure, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return err
	}
	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return err
	}
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	ocispecPlatforms, err := platformutil.NewOCISpecPlatformSlice(allPlatforms, platform)
	if err != nil {
		return err
	}

	_, err = imgutil.EnsureImage(ctx, client, os.Stdout, snapshotter, args[0],
		"always", insecure, ocispecPlatforms)
	return err
}
