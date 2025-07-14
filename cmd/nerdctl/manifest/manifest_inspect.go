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

package manifest

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/manifest"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func InspectCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "inspect MANIFEST",
		Short:             "Display the contents of a manifest",
		Args:              cobra.MinimumNArgs(1),
		RunE:              inspectAction,
		ValidArgsFunction: inspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().Bool("verbose", false, "Verbose output additional info including layers and platform")

	return cmd
}

func processInspectFlags(cmd *cobra.Command) (types.ManifestInspectOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ManifestInspectOptions{}, err
	}
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return types.ManifestInspectOptions{}, err
	}
	return types.ManifestInspectOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		Verbose:  verbose,
	}, nil
}

func inspectAction(cmd *cobra.Command, args []string) error {
	inspectOptions, err := processInspectFlags(cmd)
	if err != nil {
		return err
	}
	rawRef := args[0]

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), inspectOptions.GOptions.Namespace, inspectOptions.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	res, err := manifest.Inspect(ctx, client, rawRef, inspectOptions)
	if err != nil {
		return err
	}

	// Handle Docker-compatible output format
	// Tag non-verbose: single object
	// Tag verbose: array of objects
	// Digest non-verbose: single object
	// Digest verbose: single object
	parsedRef, parseErr := referenceutil.Parse(rawRef)
	isTagQuery := parseErr == nil && parsedRef.Digest == ""

	if isTagQuery && inspectOptions.Verbose {
		// Tag verbose case: output array
		if formatErr := formatter.FormatSlice("", inspectOptions.Stdout, res); formatErr != nil {
			log.G(ctx).Error(formatErr)
		}
	} else if len(res) == 1 {
		// Tag non-verbose or digest queries (both verbose and non-verbose): output single object
		jsonStr, err := formatter.ToJSON(res[0], "", "    ")
		if err != nil {
			log.G(ctx).Error(err)
		} else {
			fmt.Fprint(inspectOptions.Stdout, jsonStr)
		}
	} else {
		// Fallback: output array (shouldn't happen with current logic)
		if formatErr := formatter.FormatSlice("", inspectOptions.Stdout, res); formatErr != nil {
			log.G(ctx).Error(formatErr)
		}
	}
	return nil
}

func inspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}
