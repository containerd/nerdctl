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

	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func newVolumeCreateCommand() *cobra.Command {
	volumeCreateCommand := &cobra.Command{
		Use:           "create [flags] VOLUME",
		Short:         "Create a volume",
		Args:          cobra.ExactArgs(1),
		RunE:          volumeCreateAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	volumeCreateCommand.Flags().StringSlice("label", nil, "Set a label on the volume")
	return volumeCreateCommand
}

func volumeCreateAction(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.Errorf("requires exactly 1 argument")
	}
	name := args[0]
	if err := identifiers.Validate(name); err != nil {
		return errors.Wrapf(err, "malformed name %s", name)
	}

	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return err
	}
	labels, err := cmd.Flags().GetStringSlice("label")
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
