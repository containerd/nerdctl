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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/spf13/cobra"
)

func newRenameCommand() *cobra.Command {
	var renameCommand = &cobra.Command{
		Use:           "rename [flags] CONTAINER [CONTAINER, ...]",
		Args:          cobra.MaximumNArgs(2),
		Short:         "Rename container",
		RunE:          renameAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return renameCommand
}
func renameAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	var found bool
	oldName := args[0]
	newName := args[1]

	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			contLabel, err := found.Container.Labels(ctx)
			if err != nil {
				return err
			}
			err = fmt.Errorf("error when allocating new name: The container name %q is already in use by container %q. You have to remove (or rename) that container to be able to reuse that name", contLabel["nerdctl/name"], found.Container.ID())
			return err
		},
	}
	_, err = walker.Walk(ctx, newName)
	if err != nil {
		return err
	}
	for _, container := range containers {
		labels, err := container.Labels(ctx)
		if err != nil {
			return err
		}
		if labels["nerdctl/name"] == oldName {
			found = true
			if labels["nerdctl/name"] == newName {
				err = fmt.Errorf("failed to rename container named %q", newName)
				return fmt.Errorf("renaming a container with the same name as its current name:%w", err)
			}
			labels["nerdctl/name"] = newName
			_, err := container.SetLabels(ctx, labels)
			if err != nil {
				return err
			}
			err = os.Rename(filepath.Join(dataStore, "names", ns, oldName), filepath.Join(dataStore, "names", ns, newName))
			if err != nil {
				fmt.Println("error in renaming container file")
				return err
			}

			hostsstorePath := hostsstore.HostsPath(dataStore, ns, container.ID())
			metafile, err := os.ReadFile(hostsstorePath)
			if err != nil {
				fmt.Print(err)
				return nil
			}
			metafile = bytes.Replace(metafile, []byte(oldName), []byte(newName), 1)

			err = os.WriteFile(hostsstorePath, metafile, 0777)
			if err != nil {
				return err
			}

		}

	}
	if !found {
		return fmt.Errorf("container name not found:%q", oldName)
	}
	return nil
}
