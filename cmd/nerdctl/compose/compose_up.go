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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
)

func upCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "up [flags] [SERVICE...]",
		Short:         "Create and start containers",
		RunE:          upAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().Bool("abort-on-container-exit", false, "Stops all containers if any container was stopped. Incompatible with -d.")
	cmd.Flags().BoolP("detach", "d", false, "Detached mode: Run containers in the background. Incompatible with --abort-on-container-exit.")
	cmd.Flags().Bool("no-build", false, "Don't build an image, even if it's missing.")
	cmd.Flags().Bool("no-color", false, "Produce monochrome output")
	cmd.Flags().Bool("no-log-prefix", false, "Don't print prefix in logs")
	cmd.Flags().Bool("build", false, "Build images before starting containers.")
	cmd.Flags().Bool("ipfs", false, "Allow pulling base images from IPFS during build")
	cmd.Flags().Bool("quiet-pull", false, "Pull without printing progress information")
	cmd.Flags().Bool("remove-orphans", false, "Remove containers for services not defined in the Compose file.")
	cmd.Flags().Bool("force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
	cmd.Flags().Bool("no-recreate", false, "Don't recreate containers if they exist, conflict with --force-recreate.")
	cmd.Flags().StringArray("scale", []string{}, "Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.")
	cmd.Flags().String("pull", "", "Pull image before running (\"always\"|\"missing\"|\"never\")")
	return cmd
}

func upAction(cmd *cobra.Command, services []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	detach, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}
	abortOnContainerExit, err := cmd.Flags().GetBool("abort-on-container-exit")
	if detach && abortOnContainerExit {
		return fmt.Errorf("--abort-on-container-exit flag is incompatible with flag --detach")
	}
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
	pull, err := cmd.Flags().GetString("pull")
	if err != nil {
		return err
	}
	removeOrphans, err := cmd.Flags().GetBool("remove-orphans")
	if err != nil {
		return err
	}
	scaleSlice, err := cmd.Flags().GetStringArray("scale")
	if err != nil {
		return err
	}
	forceRecreate, err := cmd.Flags().GetBool("force-recreate")
	if err != nil {
		return err
	}
	noRecreate, err := cmd.Flags().GetBool("no-recreate")
	if err != nil {
		return err
	}
	if forceRecreate && noRecreate {
		return errors.New("flag --force-recreate and --no-recreate cannot be specified together")
	}
	scale := make(map[string]int)
	for _, s := range scaleSlice {
		parts := strings.Split(s, "=")
		if len(parts) != 2 {
			return fmt.Errorf("invalid --scale option %q. Should be SERVICE=NUM", s)
		}
		replicas, err := strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
		scale[parts[0]] = replicas
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	options.Services = services
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	uo := composer.UpOptions{
		AbortOnContainerExit: abortOnContainerExit,
		Detach:               detach,
		NoBuild:              noBuild,
		NoColor:              noColor,
		NoLogPrefix:          noLogPrefix,
		ForceBuild:           build,
		IPFS:                 enableIPFS,
		QuietPull:            quietPull,
		RemoveOrphans:        removeOrphans,
		Scale:                scale,
		Pull:                 pull,
		ForceRecreate:        forceRecreate,
		NoRecreate:           noRecreate,
	}
	return c.Up(ctx, uo, services)
}
