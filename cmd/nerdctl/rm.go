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
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/namestore"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newRmCommand() *cobra.Command {
	var rmCommand = &cobra.Command{
		Use:               "rm [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Remove one or more containers",
		RunE:              rmAction,
		ValidArgsFunction: rmShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	rmCommand.Flags().BoolP("force", "f", false, "Force the removal of a running|paused|unknown container (uses SIGKILL)")
	rmCommand.Flags().BoolP("volumes", "v", false, "Remove volumes associated with the container")
	return rmCommand
}

func rmAction(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	removeAnonVolumes, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return err
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	containerNameStore, err := namestore.New(dataStore, ns)
	if err != nil {
		return err
	}

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			stateDir, err := getContainerStateDirPath(cmd, dataStore, found.Container.ID())
			if err != nil {
				return err
			}
			err = removeContainer(cmd, ctx, client, ns, found.Container.ID(), found.Req, force, dataStore, stateDir, containerNameStore, removeAnonVolumes)
			return err
		},
	}
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("no such container %s", req)
		}
	}
	return nil
}

// removeContainer returns nil when the container cannot be found
// FIXME: refactoring
func removeContainer(cmd *cobra.Command, ctx context.Context, client *containerd.Client, ns, id, req string, force bool, dataStore, stateDir string, namst namestore.NameStore, removeAnonVolumes bool) (retErr error) {
	var name string
	defer func() {
		if errdefs.IsNotFound(retErr) {
			retErr = nil
		}
		if retErr == nil {
			retErr = os.RemoveAll(stateDir)
		} else {
			logrus.WithError(retErr).Warnf("failed to remove container %q", id)
		}
		if retErr == nil {
			if name != "" {
				retErr = namst.Release(name, id)
			}
		} else {
			logrus.WithError(retErr).Warnf("failed to remove container %q", id)
		}
		if retErr == nil {
			retErr = hostsstore.DeallocHostsFile(dataStore, ns, id)
		} else {
			logrus.WithError(retErr).Warnf("failed to release name store for container %q", id)
		}
	}()
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	name = l[labels.Name]
	if anonVolumesJSON, ok := l[labels.AnonymousVolumes]; ok && removeAnonVolumes {
		var anonVolumes []string
		if err := json.Unmarshal([]byte(anonVolumesJSON), &anonVolumes); err != nil {
			return err
		}
		volStore, err := getVolumeStore(cmd)
		if err != nil {
			return err
		}
		defer func() {
			if _, err := volStore.Remove(anonVolumes); err != nil {
				logrus.WithError(err).Warnf("failed to remove anonymous volumes %v", anonVolumes)
			}
		}()
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		if errdefs.IsNotFound(err) {
			if container.Delete(ctx, containerd.WithSnapshotCleanup) != nil {
				return container.Delete(ctx)
			}
		}
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err
	}

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		if _, err := task.Delete(ctx); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to delete task %v: %w", id, err)
		}
	case containerd.Paused:
		if !force {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "You cannot remove a %v container %v. Unpause the container before attempting removal or force remove\n", status.Status, id)
			return err
		}
		_, err := task.Delete(ctx, containerd.WithProcessKill)
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to delete task %v: %w", id, err)
		}
	// default is the case, when status.Status = containerd.Running
	default:
		if !force {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "You cannot remove a %v container %v. Stop the container before attempting removal or force remove\n", status.Status, id)
			return err
		}
		if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
			logrus.WithError(err).Warnf("failed to send SIGKILL")
		}
		es, err := task.Wait(ctx)
		if err == nil {
			<-es
		}
		_, err = task.Delete(ctx, containerd.WithProcessKill)
		if err != nil && !errdefs.IsNotFound(err) {
			logrus.WithError(err).Warnf("failed to delete task %v", id)
		}
	}
	var delOpts []containerd.DeleteOpts
	if _, err := container.Image(ctx); err == nil {
		delOpts = append(delOpts, containerd.WithSnapshotCleanup)
	}

	if err := container.Delete(ctx, delOpts...); err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", req)
	return err
}

func rmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return shellCompleteContainerNames(cmd, nil)
}
