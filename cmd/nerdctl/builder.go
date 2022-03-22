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
