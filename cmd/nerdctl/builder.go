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
	"fmt"
	"os"
	"os/exec"

	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newBuilderCommand() *cobra.Command {
	var builderCommand = &cobra.Command{
		Annotations:   map[string]string{Category: Management},
		Use:           "builder",
		Short:         "Manage builds",
		RunE:          unknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	builderCommand.AddCommand(
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

	AddStringFlag(buildPruneCommand, "buildkit-host", nil, defaults.BuildKitHost(), "BUILDKIT_HOST", "BuildKit address")
	return buildPruneCommand
}

func builderPruneAction(cmd *cobra.Command, args []string) error {
	buildkitHost, err := getBuildkitHost(cmd)
	if err != nil {
		return err
	}
	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		return err
	}
	buildctlArgs := buildkitutil.BuildctlBaseArgs(buildkitHost)
	buildctlArgs = append(buildctlArgs, "prune")
	logrus.Debugf("running %s %v", buildctlBinary, buildctlArgs)
	buildctlCmd := exec.Command(buildctlBinary, buildctlArgs...)
	buildctlCmd.Env = os.Environ()
	buildctlCmd.Stdout = cmd.OutOrStdout()
	return buildctlCmd.Run()
}

func newBuilderDebugCommand() *cobra.Command {
	shortHelp := `Debug Dockerfile`
	var buildDebugCommand = &cobra.Command{
		Use:           "debug",
		Short:         shortHelp,
		RunE:          builderDebugAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	buildDebugCommand.Flags().StringP("file", "f", "", "Name of the Dockerfile")
	buildDebugCommand.Flags().String("target", "", "Set the target build stage to build")
	buildDebugCommand.Flags().StringArray("build-arg", nil, "Set build-time variables")
	buildDebugCommand.Flags().String("image", "", "Image to use for debugging stage")
	return buildDebugCommand
}

func builderDebugAction(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("context needs to be specified")
	}

	buildgBinary, err := exec.LookPath("buildg")
	if err != nil {
		return err
	}
	buildgArgs := []string{"debug"}
	debugLog, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return err
	} else if debugLog {
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
			buildgArgs = append(buildgArgs, "--build-arg="+v)
		}
	}

	if imageValue, err := cmd.Flags().GetString("image"); err != nil {
		return err
	} else if imageValue != "" {
		buildgArgs = append(buildgArgs, "--image="+imageValue)
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
