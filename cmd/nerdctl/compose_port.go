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
	"strconv"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
)

func newComposePortCommand() *cobra.Command {
	var composePortCommand = &cobra.Command{
		Use:           "port [flags] SERVICE PRIVATE_PORT",
		Short:         "Print the public port for a port binding",
		Args:          cobra.ExactArgs(2),
		RunE:          composePortAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composePortCommand.Flags().Int("index", 1, "index of the container if the service has multiple instances.")
	composePortCommand.Flags().String("protocol", "tcp", "protocol of the port (tcp|udp)")

	return composePortCommand
}

func composePortAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	index, err := cmd.Flags().GetInt("index")
	if err != nil {
		return err
	}
	if index < 1 {
		return fmt.Errorf("index starts from 1 and should be equal or greater than 1, given index: %d", index)
	}

	protocol, err := cmd.Flags().GetString("protocol")
	if err != nil {
		return err
	}
	switch protocol {
	case "tcp", "udp":
	default:
		return fmt.Errorf("unsupported protocol: %s (only tcp and udp are supported)", protocol)
	}

	port, err := strconv.Atoi(args[1])
	if err != nil {
		return err
	}
	if port <= 0 {
		return fmt.Errorf("unexpected port: %d", port)
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

	po := composer.PortOptions{
		ServiceName: args[0],
		Index:       index,
		Port:        port,
		Protocol:    protocol,
	}

	return c.Port(ctx, cmd.OutOrStdout(), po)
}
