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

package builder

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/builder"
)

func NewBuilderCommand() *cobra.Command {
	var builderCommand = &cobra.Command{
		Annotations:   map[string]string{helpers.Category: helpers.Management},
		Use:           "builder",
		Short:         "Manage builds",
		RunE:          helpers.UnknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	builderCommand.AddCommand(
		NewBuildCommand(),
		newBuilderPruneCommand(),
		newBuilderDebugCommand(),
	)
	return builderCommand
}

func newBuilderPruneCommand() *cobra.Command {
	shortHelp := `Clean up BuildKit build cache`
	var buildPruneCommand = &cobra.Command{
		Use:           "prune",
		Args:          cobra.NoArgs,
		Short:         shortHelp,
		RunE:          builderPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	helpers.AddStringFlag(buildPruneCommand, "buildkit-host", nil, "", "BUILDKIT_HOST", "BuildKit address")

	buildPruneCommand.Flags().BoolP("all", "a", false, "Remove all unused build cache, not just dangling ones")
	buildPruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return buildPruneCommand
}

func builderPruneAction(cmd *cobra.Command, _ []string) error {
	options, err := processBuilderPruneOptions(cmd)
	if err != nil {
		return err
	}

	if !options.Force {
		var msg string

		if options.All {
			msg = "This will remove all build cache."
		} else {
			msg = "This will remove any dangling build cache."
		}

		if confirmed, err := helpers.Confirm(cmd, fmt.Sprintf("WARNING! %s.", msg)); err != nil || !confirmed {
			return err
		}
	}

	prunedObjects, err := builder.Prune(cmd.Context(), options)
	if err != nil {
		return err
	}

	var totalReclaimedSpace int64

	for _, prunedObject := range prunedObjects {
		totalReclaimedSpace += prunedObject.Size
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Total:  %s\n", units.BytesSize(float64(totalReclaimedSpace)))

	return nil
}

func processBuilderPruneOptions(cmd *cobra.Command) (types.BuilderPruneOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.BuilderPruneOptions{}, err
	}

	buildkitHost, err := GetBuildkitHost(cmd, globalOptions.Namespace)
	if err != nil {
		return types.BuilderPruneOptions{}, err
	}

	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return types.BuilderPruneOptions{}, err
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return types.BuilderPruneOptions{}, err
	}

	return types.BuilderPruneOptions{
		Stderr:       cmd.OutOrStderr(),
		GOptions:     globalOptions,
		BuildKitHost: buildkitHost,
		All:          all,
		Force:        force,
	}, nil
}

func newBuilderDebugCommand() *cobra.Command {
	shortHelp := `Debug Dockerfile`
	var buildDebugCommand = &cobra.Command{
		Use:           "debug",
		Short:         shortHelp,
		PreRunE:       helpers.CheckExperimental("`nerdctl builder debug`"),
		RunE:          builderDebugAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	buildDebugCommand.Flags().StringP("file", "f", "", "Name of the Dockerfile")
	buildDebugCommand.Flags().String("target", "", "Set the target build stage to build")
	buildDebugCommand.Flags().StringArray("build-arg", nil, "Set build-time variables")
	buildDebugCommand.Flags().String("image", "", "Image to use for debugging stage")
	buildDebugCommand.Flags().StringArray("ssh", nil, "Allow forwarding SSH agent to the build. Format: default|<id>[=<socket>|<key>[,<key>]]")
	buildDebugCommand.Flags().StringArray("secret", nil, "Expose secret value to the build. Format: id=secretname,src=filepath")
	return buildDebugCommand
}

func builderDebugAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	if len(args) < 1 {
		return fmt.Errorf("context needs to be specified")
	}

	buildgBinary, err := exec.LookPath("buildg")
	if err != nil {
		return err
	}
	buildgArgs := []string{"debug"}
	if globalOptions.Debug {
		buildgArgs = append([]string{"--debug"}, buildgArgs...)
	}

	if file, err := cmd.Flags().GetString("file"); err != nil {
		return err
	} else if file != "" {
		buildgArgs = append(buildgArgs, "--file="+file)
	}

	if target, err := cmd.Flags().GetString("target"); err != nil {
		return err
	} else if target != "" {
		buildgArgs = append(buildgArgs, "--target="+target)
	}

	if buildArgsValue, err := cmd.Flags().GetStringArray("build-arg"); err != nil {
		return err
	} else if len(buildArgsValue) > 0 {
		for _, v := range buildArgsValue {
			arr := strings.Split(v, "=")
			if len(arr) == 1 && len(arr[0]) > 0 {
				// Avoid masking default build arg value from Dockerfile if environment variable is not set
				// https://github.com/moby/moby/issues/24101
				val, ok := os.LookupEnv(arr[0])
				if ok {
					buildgArgs = append(buildgArgs, fmt.Sprintf("--build-arg=%s=%s", v, val))
				}
			} else if len(arr) > 1 && len(arr[0]) > 0 {
				buildgArgs = append(buildgArgs, "--build-arg="+v)
			} else {
				return fmt.Errorf("invalid build arg %q", v)
			}
		}
	}

	if imageValue, err := cmd.Flags().GetString("image"); err != nil {
		return err
	} else if imageValue != "" {
		buildgArgs = append(buildgArgs, "--image="+imageValue)
	}

	if sshValue, err := cmd.Flags().GetStringArray("ssh"); err != nil {
		return err
	} else if len(sshValue) > 0 {
		for _, v := range sshValue {
			buildgArgs = append(buildgArgs, "--ssh="+v)
		}
	}

	if secretValue, err := cmd.Flags().GetStringArray("secret"); err != nil {
		return err
	} else if len(secretValue) > 0 {
		for _, v := range secretValue {
			buildgArgs = append(buildgArgs, "--secret="+v)
		}
	}

	buildgCmd := exec.Command(buildgBinary, append(buildgArgs, args[0])...)
	buildgCmd.Env = os.Environ()
	buildgCmd.Stdin = cmd.InOrStdin()
	buildgCmd.Stdout = cmd.OutOrStdout()
	buildgCmd.Stderr = cmd.ErrOrStderr()
	if err := buildgCmd.Start(); err != nil {
		return err
	}

	return buildgCmd.Wait()
}
