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
	"context"
	"fmt"
	"os/exec"

	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/spf13/cobra"
)

func newIPFSRegistryDownCommand() *cobra.Command {
	var ipfsRegistryDownCommand = &cobra.Command{
		Use:           "down",
		Short:         "stop registry as a background container \"ipfs-registry\".",
		RunE:          ipfsRegistryDownAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	return ipfsRegistryDownCommand
}

func ipfsRegistryDownAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			return nil
		},
	}
	nc, err := walker.Walk(ctx, ipfsRegistryContainerName)
	if err != nil {
		return err
	}
	if nc == 0 {
		return fmt.Errorf("ipfs registry %q doesn't exist", ipfsRegistryContainerName)
	}
	nerdctlCmd, nerdctlArgs := globalFlags(cmd)
	if out, err := exec.Command(nerdctlCmd, append(nerdctlArgs, "stop", ipfsRegistryContainerName)...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop registry: %v: %v", string(out), err)
	}
	if out, err := exec.Command(nerdctlCmd, append(nerdctlArgs, "rm", ipfsRegistryContainerName)...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove registry: %v: %v", string(out), err)
	}
	return nil
}
