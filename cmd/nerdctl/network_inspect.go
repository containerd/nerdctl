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

	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/netutil"

	"github.com/spf13/cobra"
)

func newNetworkInspectCommand() *cobra.Command {
	networkInspectCommand := &cobra.Command{
		Use:               "inspect [flags] NETWORK [NETWORK, ...]",
		Short:             "Display detailed information on one or more networks",
		Args:              cobra.MinimumNArgs(1),
		RunE:              networkInspectAction,
		ValidArgsFunction: networkInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	networkInspectCommand.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	networkInspectCommand.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	networkInspectCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	networkInspectCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return networkInspectCommand
}

func networkInspectAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	e, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath)
	if err != nil {
		return err
	}

	netMap, err := e.NetworkMap()
	if err != nil {
		return err
	}

	result := make([]interface{}, len(args))
	for i, name := range args {
		if name == "host" || name == "none" {
			return fmt.Errorf("pseudo network %q cannot be inspected", name)
		}
		l, ok := netMap[name]
		if !ok {
			return fmt.Errorf("no such network: %s", name)
		}

		r := &native.Network{
			CNI:           json.RawMessage(l.Bytes),
			NerdctlID:     l.NerdctlID,
			NerdctlLabels: l.NerdctlLabels,
			File:          l.File,
		}
		mode, err := cmd.Flags().GetString("mode")
		if err != nil {
			return err
		}
		switch mode {
		case "native":
			result[i] = r
		case "dockercompat":
			compat, err := dockercompat.NetworkFromNative(r)
			if err != nil {
				return err
			}
			result[i] = compat
		default:
			return fmt.Errorf("unknown mode %q", mode)
		}
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	return formatter.FormatSlice(format, cmd.OutOrStdout(), result)
}

func networkInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show network names, including "bridge"
	exclude := []string{"host", "none"}
	return shellCompleteNetworkNames(cmd, exclude)
}
