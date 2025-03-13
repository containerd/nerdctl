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

package volume

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/volume"
)

func createCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "create [flags] [VOLUME]",
		Short:         "Create a volume",
		Args:          cobra.MaximumNArgs(1),
		RunE:          createAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().StringArray("label", nil, "Set a label on the volume")
	return cmd
}

func createOptions(cmd *cobra.Command) (types.VolumeCreateOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.VolumeCreateOptions{}, err
	}
	labels, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return types.VolumeCreateOptions{}, err
	}
	for _, label := range labels {
		if label == "" {
			return types.VolumeCreateOptions{}, fmt.Errorf("labels cannot be empty (%w)", errdefs.ErrInvalidArgument)
		}
	}

	return types.VolumeCreateOptions{
		GOptions: globalOptions,
		Labels:   labels,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func createAction(cmd *cobra.Command, args []string) error {
	options, err := createOptions(cmd)
	if err != nil {
		return err
	}
	volumeName := ""
	if len(args) > 0 {
		volumeName = args[0]
	}
	_, err = volume.Create(volumeName, options)

	return err
}
