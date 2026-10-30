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
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
)

func scaleCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "scale [flags] SERVICE=REPLICAS [SERVICE=REPLICAS...]",
		Short:         "Set the number of containers for services",
		Args:          cobra.MinimumNArgs(1),
		RunE:          scaleAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().Bool("no-deps", false, "Don't start linked services")
	return cmd
}

func parseScaleArgs(args []string) (map[string]int, error) {
	scale := make(map[string]int, len(args))
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid argument %q, should be SERVICE=REPLICAS", arg)
		}
		replicas, err := strconv.Atoi(value)
		if err != nil || replicas < 0 {
			return nil, fmt.Errorf("invalid number of replicas %q for service %q", value, name)
		}
		scale[name] = replicas
	}
	return scale, nil
}

func scaleAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	noDeps, err := cmd.Flags().GetBool("no-deps")
	if err != nil {
		return err
	}
	scale, err := parseScaleArgs(args)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	return c.Scale(ctx, composer.ScaleOptions{Scale: scale, NoDeps: noDeps})
}
