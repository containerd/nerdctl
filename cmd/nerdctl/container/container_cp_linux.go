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

package container

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func newCpCommand() *cobra.Command {
	shortHelp := "Copy files/folders between a running container and the local filesystem."

	longHelp := shortHelp + `
This command requires 'tar' to be installed on the host (not in the container).
Using GNU tar is recommended.
The path of the 'tar' binary can be specified with an environment variable '$TAR'.

WARNING: 'nerdctl cp' is designed only for use with trusted, cooperating containers.
Using 'nerdctl cp' with untrusted or malicious containers is unsupported and may not provide protection against unexpected behavior.
`

	usage := `cp [flags] CONTAINER:SRC_PATH DEST_PATH|-
  nerdctl cp [flags] SRC_PATH|- CONTAINER:DEST_PATH`
	var cpCommand = &cobra.Command{
		Use:               usage,
		Args:              helpers.IsExactArgs(2),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              cpAction,
		ValidArgsFunction: cpShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	cpCommand.Flags().BoolP("follow-link", "L", false, "Always follow symbolic link in SRC_PATH.")

	return cpCommand
}

func cpAction(cmd *cobra.Command, args []string) error {
	options, err := processCpOptions(cmd, args)
	if err != nil {
		return err
	}
	if rootlessutil.IsRootless() {
		options.GOptions.Address, err = rootlessutil.RootlessContainredSockAddress()
		if err != nil {
			return err
		}
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Cp(ctx, client, options)
}

func processCpOptions(cmd *cobra.Command, args []string) (types.ContainerCpOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerCpOptions{}, err
	}
	flagL, err := cmd.Flags().GetBool("follow-link")
	if err != nil {
		return types.ContainerCpOptions{}, err
	}

	srcSpec, err := parseCpFileSpec(args[0])
	if err != nil {
		return types.ContainerCpOptions{}, err
	}

	destSpec, err := parseCpFileSpec(args[1])
	if err != nil {
		return types.ContainerCpOptions{}, err
	}

	if (srcSpec.Container != nil && destSpec.Container != nil) || (len(srcSpec.Path) == 0 && len(destSpec.Path) == 0) {
		return types.ContainerCpOptions{}, fmt.Errorf("one of src or dest must be a local file specification")
	}
	if srcSpec.Container == nil && destSpec.Container == nil {
		return types.ContainerCpOptions{}, fmt.Errorf("one of src or dest must be a container file specification")
	}
	if srcSpec.Path == "-" {
		return types.ContainerCpOptions{}, fmt.Errorf("support for reading a tar archive from stdin is not implemented yet")
	}
	if destSpec.Path == "-" {
		return types.ContainerCpOptions{}, fmt.Errorf("support for writing a tar archive to stdout is not implemented yet")
	}

	container2host := srcSpec.Container != nil
	var container string
	if container2host {
		container = *srcSpec.Container
	} else {
		container = *destSpec.Container
	}
	return types.ContainerCpOptions{
		GOptions:       globalOptions,
		Container2Host: container2host,
		ContainerReq:   container,
		DestPath:       destSpec.Path,
		SrcPath:        srcSpec.Path,
		FollowSymLink:  flagL,
	}, nil
}

func AddCpCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newCpCommand())
}

var errFileSpecDoesntMatchFormat = errors.New("filespec must match the canonical format: [container:]file/path")

func parseCpFileSpec(arg string) (*cpFileSpec, error) {
	i := strings.Index(arg, ":")

	// filespec starting with a semicolon is invalid
	if i == 0 {
		return nil, errFileSpecDoesntMatchFormat
	}

	if filepath.IsAbs(arg) {
		// Explicit local absolute path, e.g., `C:\foo` or `/foo`.
		return &cpFileSpec{
			Container: nil,
			Path:      arg,
		}, nil
	}

	parts := strings.SplitN(arg, ":", 2)

	if len(parts) == 1 || strings.HasPrefix(parts[0], ".") {
		// Either there's no `:` in the arg
		// OR it's an explicit local relative path like `./file:name.txt`.
		return &cpFileSpec{
			Path: arg,
		}, nil
	}

	return &cpFileSpec{
		Container: &parts[0],
		Path:      parts[1],
	}, nil
}

type cpFileSpec struct {
	Container *string
	Path      string
}

func cpShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterFileExt
}
