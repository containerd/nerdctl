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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/spf13/cobra"
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
		Args:              IsExactArgs(2),
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

	return container.Cp(cmd.Context(), options)
}

func processCpOptions(cmd *cobra.Command, args []string) (types.ContainerCpOptions, error) {
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
	ctx := cmd.Context()

	// cp works in the host namespace (for inspecting file permissions), so we can't directly use the Go client.

	selfExe, inspectArgs := globalFlags(cmd)
	inspectArgs = append(inspectArgs, "container", "inspect", "--mode=native", "--format={{json .Process}}", container)
	inspectCmd := exec.CommandContext(ctx, selfExe, inspectArgs...)
	inspectCmd.Stderr = os.Stderr
	inspectOut, err := inspectCmd.Output()
	if err != nil {
		return types.ContainerCpOptions{}, fmt.Errorf("failed to execute %v: %w", inspectCmd.Args, err)
	}
	var proc native.Process
	if err := json.Unmarshal(inspectOut, &proc); err != nil {
		return types.ContainerCpOptions{}, err
	}
	if proc.Status.Status != containerd.Running {
		return types.ContainerCpOptions{}, fmt.Errorf("expected container status %v, got %v", containerd.Running, proc.Status.Status)
	}
	if proc.Pid <= 0 {
		return types.ContainerCpOptions{}, fmt.Errorf("got non-positive PID %v", proc.Pid)
	}

	return types.ContainerCpOptions{
		Container2Host: container2host,
		Pid:            proc.Pid,
		DestPath:       destSpec.Path,
		SrcPath:        srcSpec.Path,
		FollowSymLink:  flagL,
	}, nil
}
