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

package compose

import (
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeBuildCommand() *cobra.Command {
	var composeBuildCommand = &cobra.Command{
		Use:           "build [flags] [SERVICE...]",
		Short:         "Build or rebuild services",
		RunE:          composeBuildAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeBuildCommand.Flags().StringArray("build-arg", nil, "Set build-time variables for services.")
	composeBuildCommand.Flags().Bool("no-cache", false, "Do not use cache when building the image.")
	composeBuildCommand.Flags().String("progress", "", "Set type of progress output (auto, plain, tty). Use plain to show container output")

	composeBuildCommand.Flags().Bool("ipfs", false, "Allow pulling base images from IPFS during build")

	return composeBuildCommand
}

func composeBuildAction(cmd *cobra.Command, args []string) error {
	buildArg, err := cmd.Flags().GetStringArray("build-arg")
	if err != nil {
		return err
	}
	noCache, err := cmd.Flags().GetBool("no-cache")
	if err != nil {
		return err
	}
	progress, err := cmd.Flags().GetString("progress")
	if err != nil {
		return err
	}
	enableIPFS, err := cmd.Flags().GetBool("ipfs")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := ncclient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}
	bo := composer.BuildOptions{
		Args:     buildArg,
		NoCache:  noCache,
		Progress: progress,
		IPFS:     enableIPFS,
	}
	return c.Build(ctx, bo, args)
}
