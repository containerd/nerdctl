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
	"context"

	composecli "github.com/compose-spec/compose-go/cli"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func newComposeCommand() *cobra.Command {
	var composeCommand = &cobra.Command{
		Use:           "compose",
		Short:         "Compose",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeCommand.PersistentFlags().StringP("file", "f", "", "Specify an alternate compose file")
	composeCommand.PersistentFlags().String("project-directory", "", "Specify an alternate working directory")
	composeCommand.PersistentFlags().StringP("project-name", "p", "", "Specify an alternate project name")
	composeCommand.PersistentFlags().String("env-file", "", "Specify an alternate environment file")

	composeCommand.AddCommand(
		newComposeUpCommand(),
		newComposeLogsCommand(),
		newComposeBuildCommand(),
		newComposeDownCommand(),
		newComposePsCommand(),
	)

	return composeCommand
}

func getComposer(cmd *cobra.Command, client *containerd.Client) (*composer.Composer, error) {
	nerdctlCmd, nerdctlArgs := globalFlags(cmd)
	projectDirectory, err := cmd.Flags().GetString("project-directory")
	if err != nil {
		return nil, err
	}
	envFile, err := cmd.Flags().GetString("env-file")
	if err != nil {
		return nil, err
	}
	projectName, err := cmd.Flags().GetString("project-name")
	if err != nil {
		return nil, err
	}
	debugFull, err := cmd.Flags().GetBool("debug-full")
	if err != nil {
		return nil, err
	}
	file, err := cmd.Flags().GetString("file")
	if err != nil {
		return nil, err
	}
	insecure, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return nil, err
	}
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return nil, err
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return nil, err
	}
	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return nil, err
	}
	o := composer.Options{
		ProjectOptions: composecli.ProjectOptions{
			WorkingDir:  projectDirectory,
			ConfigPaths: []string{},
			Environment: map[string]string{},
			EnvFile:     envFile,
		},
		Project:        projectName,
		NerdctlCmd:     nerdctlCmd,
		NerdctlArgs:    nerdctlArgs,
		DebugPrintFull: debugFull,
	}

	if file != "" {
		o.ProjectOptions.ConfigPaths = append([]string{file}, o.ProjectOptions.ConfigPaths...)
	}
	cniEnv := &netutil.CNIEnv{
		Path:        cniPath,
		NetconfPath: cniNetconfpath,
	}
	configLists, err := netutil.ConfigLists(cniEnv)
	if err != nil {
		return nil, err
	}

	o.NetworkExists = func(netName string) (bool, error) {
		for _, f := range configLists {
			if f.Name == netName {
				return true, nil
			}
		}
		return false, nil
	}

	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return nil, err
	}

	o.VolumeExists = func(volName string) (bool, error) {
		if _, volGetErr := volStore.Get(volName); volGetErr == nil {
			return true, nil
		} else if errors.Is(volGetErr, errdefs.ErrNotFound) {
			return false, nil
		} else {
			return false, volGetErr
		}
	}

	o.ImageExists = func(ctx context.Context, rawRef string) (bool, error) {
		named, err := refdocker.ParseDockerRef(rawRef)
		if err != nil {
			return false, err
		}
		ref := named.String()
		if _, err := client.ImageService().Get(ctx, ref); err != nil {
			if errors.Is(err, errdefs.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	o.EnsureImage = func(ctx context.Context, imageName, pullMode string) error {
		_, imgErr := imgutil.EnsureImage(ctx, client, cmd.OutOrStdout(), snapshotter, imageName,
			pullMode, insecure)
		return imgErr
	}

	return composer.New(o)
}
