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
	types "github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/volume"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"

	"github.com/spf13/cobra"
)

func newVolumeLsCommand() *cobra.Command {
	volumeLsCommand := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List volumes",
		RunE:          volumeLsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	volumeLsCommand.Flags().BoolP("quiet", "q", false, "Only display volume names")
	// Alias "-f" is reserved for "--filter"
	volumeLsCommand.Flags().String("format", "", "Format the output using the given go template")
	volumeLsCommand.Flags().BoolP("size", "s", false, "Display the disk usage of volumes. Can be slow with volumes having loads of directories.")
	volumeLsCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	volumeLsCommand.Flags().StringSliceP("filter", "f", []string{}, "Filter matches volumes based on given conditions")
	return volumeLsCommand
}

func volumeLsAction(cmd *cobra.Command, args []string) error {
	options := &types.VolumeLsCommandOptions{}
	options.Writer = cmd.OutOrStdout()
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	options.Quiet = quiet
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	options.Format = format
	size, err := cmd.Flags().GetBool("size")
	if err != nil {
		return err
	}
	options.Size = size
	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return err
	}
	options.Filters = filters
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	options.Namespace = ns
	dataRoot, err := cmd.Flags().GetString("data-root")
	if err != nil {
		return err
	}
	options.DataRoot = dataRoot
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return err
	}
	options.Address = address
	return volume.Ls(options)
}

func getVolumes(cmd *cobra.Command) (map[string]native.Volume, error) {
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, err
	}
	dataRoot, err := cmd.Flags().GetString("data-root")
	if err != nil {
		return nil, err
	}
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return nil, err
	}
	volumeSize, err := cmd.Flags().GetBool("size")
	if err != nil {
		return nil, err
	}
	return volume.Volumes(ns, dataRoot, address, volumeSize)
}
