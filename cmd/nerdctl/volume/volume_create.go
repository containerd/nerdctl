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

	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/containerd/nerdctl/pkg/strutil"

	"github.com/spf13/cobra"
)

func NewCreateCommand() *cobra.Command {
	volumeCreateCommand := &cobra.Command{
		Use:           "create [flags] VOLUME",
		Short:         "Create a volume",
		Args:          utils.IsExactArgs(1),
		RunE:          volumeCreateAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	volumeCreateCommand.Flags().StringArray("label", nil, "Set a label on the volume")
	return volumeCreateCommand
}

func volumeCreateAction(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := identifiers.Validate(name); err != nil {
		return fmt.Errorf("malformed name %s: %w", name, err)
	}

	volStore, err := volume.GetVolumeStore(cmd)
	if err != nil {
		return err
	}
	labels, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return err
	}
	labels = strutil.DedupeStrSlice(labels)
	if _, err := volStore.Create(name, labels); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", name)
	return nil
}
