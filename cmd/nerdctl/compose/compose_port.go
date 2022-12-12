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

	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
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

	client, ctx, cancel, err := nerdClient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
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
