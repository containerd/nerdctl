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

package compose

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/pkg/version"
)

func versionCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "version",
		Short:         "Show the Compose version information",
		Args:          cobra.NoArgs,
		RunE:          versionAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().StringP("format", "f", "pretty", "Format the output. Values: [pretty | json]")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "pretty"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().Bool("short", false, "Shows only Compose's version number")
	return cmd
}

func versionAction(cmd *cobra.Command, args []string) error {
	short, err := cmd.Flags().GetBool("short")
	if err != nil {
		return err
	}
	if short {
		fmt.Fprintln(cmd.OutOrStdout(), strings.TrimPrefix(version.GetVersion(), "v"))
		return nil
	}

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "pretty":
		fmt.Fprintln(cmd.OutOrStdout(), "nerdctl Compose version "+version.GetVersion())
	case "json":
		fmt.Fprintf(cmd.OutOrStdout(), "{\"version\":\"%v\"}\n", version.Version)
	default:
		return fmt.Errorf("format can be either pretty or json, not %v", format)
	}

	return nil
}
