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
	"strings"

	"github.com/containerd/nerdctl/pkg/version"
	"github.com/spf13/cobra"
)

func newComposeVersionCommand() *cobra.Command {
	var composeVersionCommand = &cobra.Command{
		Use:           "version",
		Short:         "Show the Compose version information",
		Args:          cobra.NoArgs,
		RunE:          composeVersionAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeVersionCommand.Flags().StringP("format", "f", "pretty", "Format the output. Values: [pretty | json]")
	composeVersionCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "pretty"}, cobra.ShellCompDirectiveNoFileComp
	})
	composeVersionCommand.Flags().Bool("short", false, "Shows only Compose's version number")
	return composeVersionCommand
}

func composeVersionAction(cmd *cobra.Command, args []string) error {
	short, err := cmd.Flags().GetBool("short")
	if err != nil {
		return err
	}
	if short {
		fmt.Fprintln(cmd.OutOrStdout(), strings.TrimPrefix(version.Version, "v"))
		return nil
	}

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "pretty":
		fmt.Fprintln(cmd.OutOrStdout(), "nerdctl Compose version "+version.Version)
	case "json":
		fmt.Fprintf(cmd.OutOrStdout(), "{\"version\":\"%v\"}\n", version.Version)
	default:
		return fmt.Errorf("format can be either pretty or json, not %v", format)
	}

	return nil
}
