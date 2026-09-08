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

package system

import (
	"github.com/spf13/cobra"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/builder"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/system"
)

func dfCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "df [flags]",
		Short:         "Show nerdctl disk usage",
		Args:          cobra.NoArgs,
		RunE:          dfAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("verbose", "v", false, "Show detailed information on space usage")
	cmd.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	return cmd
}

func dfOptions(cmd *cobra.Command) (types.SystemDfOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.SystemDfOptions{}, err
	}

	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return types.SystemDfOptions{}, err
	}

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.SystemDfOptions{}, err
	}

	buildkitHost, err := builder.GetBuildkitHost(cmd, globalOptions.Namespace)
	if err != nil {
		log.L.WithError(err).Warn("BuildKit is not running. The build cache usage will be reported as empty.")
		buildkitHost = ""
	}

	return types.SystemDfOptions{
		Stdout:       cmd.OutOrStdout(),
		Stderr:       cmd.ErrOrStderr(),
		GOptions:     globalOptions,
		Format:       format,
		Verbose:      verbose,
		BuildKitHost: buildkitHost,
	}, nil
}

func dfAction(cmd *cobra.Command, _ []string) error {
	options, err := dfOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return system.Df(ctx, client, options)
}
