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
	"strconv"
	"strings"

	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeUpCommand() *cobra.Command {
	var composeUpCommand = &cobra.Command{
		Use:           "up [SERVICE...]",
		Short:         "Create and start containers",
		RunE:          composeUpAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeUpCommand.Flags().BoolP("detach", "d", false, "Detached mode: Run containers in the background")
	composeUpCommand.Flags().Bool("no-build", false, "Don't build an image, even if it's missing.")
	composeUpCommand.Flags().Bool("no-color", false, "Produce monochrome output")
	composeUpCommand.Flags().Bool("no-log-prefix", false, "Don't print prefix in logs")
	composeUpCommand.Flags().Bool("build", false, "Build images before starting containers.")
	composeUpCommand.Flags().Bool("ipfs", false, "Allow pulling base images from IPFS during build")
	composeUpCommand.Flags().Bool("quiet-pull", false, "Pull without printing progress information")
	composeUpCommand.Flags().StringArray("scale", []string{}, "Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.")
	return composeUpCommand
}

func composeUpAction(cmd *cobra.Command, services []string) error {
	detach, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}
	noBuild, err := cmd.Flags().GetBool("no-build")
	if err != nil {
		return err
	}
	noColor, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return err
	}
	noLogPrefix, err := cmd.Flags().GetBool("no-log-prefix")
	if err != nil {
		return err
	}
	build, err := cmd.Flags().GetBool("build")
	if err != nil {
		return err
	}
	if build && noBuild {
		return errors.New("--build and --no-build can not be combined")
	}
	enableIPFS, err := cmd.Flags().GetBool("ipfs")
	if err != nil {
		return err
	}
	quietPull, err := cmd.Flags().GetBool("quiet-pull")
	if err != nil {
		return err
	}
	scaleSlice, err := cmd.Flags().GetStringArray("scale")
	if err != nil {
		return err
	}
	scale := make(map[string]uint64)
	for _, s := range scaleSlice {
		parts := strings.Split(s, "=")
		if len(parts) != 2 {
			return fmt.Errorf("invalid --scale option %q. Should be SERVICE=NUM", s)
		}
		replicas, err := strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
		scale[parts[0]] = uint64(replicas)
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}
	uo := composer.UpOptions{
		Detach:      detach,
		NoBuild:     noBuild,
		NoColor:     noColor,
		NoLogPrefix: noLogPrefix,
		ForceBuild:  build,
		IPFS:        enableIPFS,
		QuietPull:   quietPull,
		Scale:       scale,
	}
	return c.Up(ctx, uo, services)
}
