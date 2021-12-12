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
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type renameOptions struct {
	oldName   string
	newName   string
	ns        string
	dataStore string
}

func newRenameCommand() *cobra.Command {
	var renameCommand = &cobra.Command{
		Use:   "rename CONTAINER NEW_NAME",
		Args:  cobra.MinimumNArgs(2),
		Short: "Rename a container",
		RunE: func(cmd *cobra.Command, args []string) error {
			var opts renameOptions
			opts.oldName = args[0]
			opts.newName = args[1]
			if err := identifiers.Validate(opts.newName); err != nil {
				return fmt.Errorf("invalid name %q: %w", opts.newName, err)
			}
			ns, err := cmd.Flags().GetString("namespace")
			if err != nil {
				return err
			}
			opts.ns = ns
			opts.dataStore, err = getDataStore(cmd)
			if err != nil {
				return err
			}
			return renameAction(cmd, &opts)
		},
		ValidArgsFunction: renameShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return renameCommand
}

func renameAction(cmd *cobra.Command, opts *renameOptions) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Container.ID())
			}
			if err := renameContainer(ctx, client, found.Container.ID(), opts); err != nil {
				return err
			}
			return nil
		},
	}
	n, err := walker.Walk(ctx, opts.oldName)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", opts.oldName)
	}
	return nil
}

func renameContainer(ctx context.Context, client *containerd.Client, id string, opts *renameOptions) error {
	err := checkNewName(id, opts)
	if err != nil {
		return err
	}
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	lab, err := container.Labels(ctx)
	oldName, ok := lab[labels.Name]
	if ok {
		if oldName == opts.newName {
			logrus.Errorf("Renaming a container with the same name as its current name")
		}
	}
	metaDir := hostsstore.GetMetaPath(opts.dataStore, opts.ns, id)
	if err != nil {
		return err
	}
	if _, err = os.Stat(metaDir); err == nil {
		var metaData = hostsstore.Meta{}
		metaData, err = hostsstore.ReadMeta(metaDir)
		if err != nil {
			return err
		}
		metaData.Name = opts.newName
		err = hostsstore.WriteMeta(metaDir, metaData)
		if err != nil {
			return err
		}
	}
	if err = changeNamestore(opts); err != nil {
		return err
	}
	lab[labels.Name] = opts.newName
	_, err = container.SetLabels(ctx, lab)
	if err != nil {
		return err
	}
	return nil
}

func renameShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Running && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}

func checkNewName(id string, opts *renameOptions) error {
	containerNameStore, err := namestore.New(opts.dataStore, opts.ns)
	if err != nil {
		return err
	}
	if err := containerNameStore.Acquire(opts.newName, id); err != nil {
		return err
	}
	return nil
}

func changeNamestore(opts *renameOptions) error {
	containerNameStore, err := namestore.New(opts.dataStore, opts.ns)
	if err != nil {
		return err
	}
	if err = containerNameStore.ChangeName(opts.oldName, opts.newName); err != nil {
		return err
	}
	return nil
}
