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

package search

import (
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/search"
)

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "search [OPTIONS] TERM",
		Short:                 "Search registry for images",
		Args:                  cobra.ExactArgs(1),
		RunE:                  runSearch,
		DisableFlagsInUseLine: true,
	}

	flags := cmd.Flags()

	flags.Bool("no-trunc", false, "Don't truncate output")
	flags.StringSliceP("filter", "f", nil, "Filter output based on conditions provided")
	flags.Int("limit", 0, "Max number of search results")
	flags.String("format", "", "Pretty-print search using a Go template")

	return cmd
}

func processSearchFlags(cmd *cobra.Command) (types.SearchOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.SearchOptions{}, err
	}

	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return types.SearchOptions{}, err
	}
	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return types.SearchOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.SearchOptions{}, err
	}
	filter, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return types.SearchOptions{}, err
	}

	return types.SearchOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		NoTrunc:  noTrunc,
		Limit:    limit,
		Filters:  filter,
		Format:   format,
	}, nil
}

func runSearch(cmd *cobra.Command, args []string) error {
	options, err := processSearchFlags(cmd)
	if err != nil {
		return err
	}

	return search.Search(cmd.Context(), args[0], options)
}
