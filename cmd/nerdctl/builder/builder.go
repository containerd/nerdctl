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
	"time"

	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/builder"
)

func Command() *cobra.Command {
	var cmd = &cobra.Command{
		Annotations:   map[string]string{helpers.Category: helpers.Management},
		Use:           "builder",
		Short:         "Manage builds",
		RunE:          helpers.UnknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		BuildCommand(),
		pruneCommand(),
		debugCommand(),
	)
	return cmd
}

func pruneCommand() *cobra.Command {
	shortHelp := `Clean up BuildKit build cache`
	var cmd = &cobra.Command{
		Use:           "prune",
		Args:          cobra.NoArgs,
		Short:         shortHelp,
		RunE:          pruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.Flags().String("buildkit-host", "", "BuildKit address")
	cmd.Flags().BoolP("all", "a", false, "Remove all unused build cache, not just dangling ones")
	cmd.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return cmd
}

func pruneAction(cmd *cobra.Command, _ []string) error {
	options, err := pruneOptions(cmd)
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

func pruneOptions(cmd *cobra.Command) (types.BuilderPruneOptions, error) {
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

func debugCommand() *cobra.Command {
	shortHelp := `Debug Dockerfile`
	var cmd = &cobra.Command{
		Use:           "debug",
		Short:         shortHelp,
		PreRunE:       helpers.CheckExperimental("`nerdctl builder debug`"),
		RunE:          debugAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().StringP("file", "f", "", "Name of the Dockerfile")
	cmd.Flags().String("target", "", "Set the target build stage to build")
	cmd.Flags().StringArray("build-arg", nil, "Set build-time variables")
	cmd.Flags().String("image", "", "Image to use for debugging stage")
	cmd.Flags().StringArray("ssh", nil, "Allow forwarding SSH agent to the build. Format: default|<id>[=<socket>|<key>[,<key>]]")
	cmd.Flags().StringArray("secret", nil, "Expose secret value to the build. Format: id=secretname,src=filepath")
	helpers.AddDurationFlag(cmd, "buildg-startup-timeout", nil, 1*time.Minute, "", "Timeout for starting up buildg")
	return cmd
}

func debugAction(cmd *cobra.Command, args []string) error {
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

	startupTimeout, err := cmd.Flags().GetDuration("buildg-startup-timeout")
	if err != nil {
		return err
	}
	buildgArgs = append(buildgArgs, "--startup-timeout="+startupTimeout.String())

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
