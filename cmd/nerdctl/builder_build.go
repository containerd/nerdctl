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
	"os"
	"strings"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/builder"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/strutil"

	"github.com/spf13/cobra"
)

func newBuildCommand() *cobra.Command {
	var buildCommand = &cobra.Command{
		Use:   "build [flags] PATH",
		Short: "Build an image from a Dockerfile. Needs buildkitd to be running.",
		Long: `Build an image from a Dockerfile. Needs buildkitd to be running.
If Dockerfile is not present and -f is not specified, it will look for Containerfile and build with it. `,
		RunE:          buildAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	AddStringFlag(buildCommand, "buildkit-host", nil, defaults.BuildKitHost(), "BUILDKIT_HOST", "BuildKit address")
	buildCommand.Flags().StringArrayP("tag", "t", nil, "Name and optionally a tag in the 'name:tag' format")
	buildCommand.Flags().StringP("file", "f", "", "Name of the Dockerfile")
	buildCommand.Flags().String("target", "", "Set the target build stage to build")
	buildCommand.Flags().StringArray("build-arg", nil, "Set build-time variables")
	buildCommand.Flags().Bool("no-cache", false, "Do not use cache when building the image")
	buildCommand.Flags().StringP("output", "o", "", "Output destination (format: type=local,dest=path)")
	buildCommand.Flags().String("progress", "auto", "Set type of progress output (auto, plain, tty). Use plain to show container output")
	buildCommand.Flags().StringArray("secret", nil, "Secret file to expose to the build: id=mysecret,src=/local/secret")
	buildCommand.Flags().StringArray("ssh", nil, "SSH agent socket or keys to expose to the build (format: default|<id>[=<socket>|<key>[,<key>]])")
	buildCommand.Flags().BoolP("quiet", "q", false, "Suppress the build output and print image ID on success")
	buildCommand.Flags().StringArray("cache-from", nil, "External cache sources (eg. user/app:cache, type=local,src=path/to/dir)")
	buildCommand.Flags().StringArray("cache-to", nil, "Cache export destinations (eg. user/app:cache, type=local,dest=path/to/dir)")
	buildCommand.Flags().Bool("rm", true, "Remove intermediate containers after a successful build")
	buildCommand.Flags().String("network", "default", "Set type of network for build (format:network=default|none|host)")
	buildCommand.RegisterFlagCompletionFunc("network", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"default", "host", "none"}, cobra.ShellCompDirectiveNoFileComp
	})
	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	buildCommand.Flags().StringSlice("platform", []string{}, "Set target platform for build (e.g., \"amd64\", \"arm64\")")
	buildCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	// #endregion

	buildCommand.Flags().String("iidfile", "", "Write the image ID to the file")
	buildCommand.Flags().StringArray("label", nil, "Set metadata for an image")

	return buildCommand
}

func processBuildCommandFlag(cmd *cobra.Command, args []string) (types.BuilderBuildOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	buildKitHost, err := getBuildkitHost(cmd, globalOptions.Namespace)
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	platform = strutil.DedupeStrSlice(platform)
	if len(args) < 1 {
		return types.BuilderBuildOptions{}, errors.New("context needs to be specified")
	}
	buildContext := args[0]
	if buildContext == "-" || strings.Contains(buildContext, "://") {
		return types.BuilderBuildOptions{}, fmt.Errorf("unsupported build context: %q", buildContext)
	}
	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	tagValue, err := cmd.Flags().GetStringArray("tag")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	progress, err := cmd.Flags().GetString("progress")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	filename, err := cmd.Flags().GetString("file")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	target, err := cmd.Flags().GetString("target")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	buildArgs, err := cmd.Flags().GetStringArray("build-arg")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	label, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	noCache, err := cmd.Flags().GetBool("no-cache")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	secret, err := cmd.Flags().GetStringArray("secret")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	ssh, err := cmd.Flags().GetStringArray("ssh")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	cacheFrom, err := cmd.Flags().GetStringArray("cache-from")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	cacheTo, err := cmd.Flags().GetStringArray("cache-to")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	rm, err := cmd.Flags().GetBool("rm")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	iidfile, err := cmd.Flags().GetString("iidfile")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	network, err := cmd.Flags().GetString("network")
	if err != nil {
		return types.BuilderBuildOptions{}, err
	}
	return types.BuilderBuildOptions{
		GOptions:     globalOptions,
		BuildKitHost: buildKitHost,
		BuildContext: buildContext,
		Output:       output,
		Tag:          tagValue,
		Progress:     progress,
		File:         filename,
		Target:       target,
		BuildArgs:    buildArgs,
		Label:        label,
		NoCache:      noCache,
		Secret:       secret,
		SSH:          ssh,
		CacheFrom:    cacheFrom,
		CacheTo:      cacheTo,
		Rm:           rm,
		IidFile:      iidfile,
		Quiet:        quiet,
		Platform:     platform,
		Stdout:       cmd.OutOrStdout(),
		Stderr:       cmd.OutOrStderr(),
		Stdin:        cmd.InOrStdin(),
		NetworkMode:  network,
	}, nil
}

func getBuildkitHost(cmd *cobra.Command, namespace string) (string, error) {
	if cmd.Flags().Changed("buildkit-host") || os.Getenv("BUILDKIT_HOST") != "" {
		// If address is explicitly specified, use it.
		buildkitHost, err := cmd.Flags().GetString("buildkit-host")
		if err != nil {
			return "", err
		}
		if err := buildkitutil.PingBKDaemon(buildkitHost); err != nil {
			return "", err
		}
		return buildkitHost, nil
	}
	return buildkitutil.GetBuildkitHost(namespace)
}

func buildAction(cmd *cobra.Command, args []string) error {
	options, err := processBuildCommandFlag(cmd, args)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return builder.Build(ctx, client, options)
}
