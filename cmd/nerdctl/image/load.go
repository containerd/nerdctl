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

package image

import (
	"errors"
	"os"

	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/spf13/cobra"
)

func NewLoadCommand() *cobra.Command {
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
	loadCommand.RegisterFlagCompletionFunc("platform", completion.ShellCompletePlatforms)
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
		if stdinStat.Size() == 0 && (stdinStat.Mode()&os.ModeNamedPipe) == 0 {
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

	return utils.LoadImage(decompressor, cmd, platMC, false)
}
