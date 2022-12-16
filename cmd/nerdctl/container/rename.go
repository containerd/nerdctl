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

package container

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/spf13/cobra"
)

func NewRenameCommand() *cobra.Command {
	var renameCommand = &cobra.Command{
		Use:               "rename [flags] CONTAINER NEW_NAME",
		Args:              utils.IsExactArgs(2),
		Short:             "rename a container",
		RunE:              renameAction,
		ValidArgsFunction: renameShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return renameCommand
}

func renameAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := ncclient.New(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	dataStore, err := ncclient.DataStore(cmd)
	if err != nil {
		return err
	}
	namest, err := namestore.New(dataStore, ns)
	if err != nil {
		return err
	}
	hostst, err := hostsstore.NewStore(dataStore)
	if err != nil {
		return err
	}
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			return renameContainer(ctx, found.Container, args[1], ns, namest, hostst)
		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

func renameContainer(ctx context.Context, container containerd.Container, newName, ns string, namst namestore.NameStore, hostst hostsstore.Store) error {
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	name := l[labels.Name]
	if err := namst.Rename(name, container.ID(), newName); err != nil {
		return err
	}
	if err := hostst.Update(ns, container.ID(), newName); err != nil {
		return err
	}
	labels := map[string]string{
		labels.Name: newName,
	}
	if _, err = container.SetLabels(ctx, labels); err != nil {
		return err
	}
	return nil
}

func renameShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ShellCompleteContainerNames(cmd, nil)
}
