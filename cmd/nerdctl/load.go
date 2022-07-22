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
	"fmt"
	"io"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/spf13/cobra"
)

func newLoadCommand() *cobra.Command {
	var loadCommand = &cobra.Command{
		Use:           "load",
		Args:          cobra.NoArgs,
		Short:         "Load an image from a tar archive or STDIN",
		Long:          "Supports both Docker Image Spec v1.2 and OCI Image Spec v1.0.",
		RunE:          loadAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	loadCommand.Flags().StringP("input", "i", "", "Read from tar archive file, instead of STDIN")

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	loadCommand.Flags().StringSlice("platform", []string{}, "Import content for a specific platform")
	loadCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	loadCommand.Flags().Bool("all-platforms", false, "Import content for all platforms")
	// #endregion

	return loadCommand
}

func loadAction(cmd *cobra.Command, _ []string) error {
	in := cmd.InOrStdin()
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	if input != "" {
		f, err := os.Open(input)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	} else {
		// check if stdin is empty.
		stdinStat, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if stdinStat.Size() == 0 {
			return errors.New("stdin is empty and input flag is not specified")
		}
	}
	decompressor, err := compression.DecompressStream(in)
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
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return err
	}

	return loadImage(decompressor, cmd, platMC, false)
}

func loadImage(in io.Reader, cmd *cobra.Command, platMC platforms.MatchComparer, quiet bool) error {
	// In addition to passing WithImagePlatform() to client.Import(), we also need to pass WithDefaultPlatform() to newClient().
	// Otherwise unpacking may fail.
	client, ctx, cancel, err := newClient(cmd, containerd.WithDefaultPlatform(platMC))
	if err != nil {
		return err
	}
	defer cancel()

	sn, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return err
	}
	imgs, err := client.Import(ctx, in, containerd.WithDigestRef(archive.DigestTranslator(sn)), containerd.WithSkipDigestRef(func(name string) bool { return name != "" }), containerd.WithImportPlatform(platMC))
	if err != nil {
		if errors.Is(err, images.ErrEmptyWalk) {
			err = fmt.Errorf("%w (Hint: set `--platform=PLATFORM` or `--all-platforms`)", err)
		}
		return err
	}
	for _, img := range imgs {
		image := containerd.NewImageWithPlatform(client, img, platMC)

		// TODO: Show unpack status
		if !quiet {
			fmt.Fprintf(cmd.OutOrStdout(), "unpacking %s (%s)...", img.Name, img.Target.Digest)
		}
		err = image.Unpack(ctx, sn)
		if err != nil {
			return err
		}
		if quiet {
			fmt.Fprintln(cmd.OutOrStdout(), img.Target.Digest)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "done\n")
		}
	}

	return nil
}
