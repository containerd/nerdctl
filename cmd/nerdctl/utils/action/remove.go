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

package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func RemoveContainer(ctx context.Context, cmd *cobra.Command, container containerd.Container, force bool, removeAnonVolumes bool) (retErr error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return err
	}

	id := container.ID()
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	stateDir := l[labels.StateDir]
	name := l[labels.Name]

	dataStore, err := client.DataStore(cmd)
	if err != nil {
		return err
	}
	namst, err := namestore.New(dataStore, ns)
	if err != nil {
		return err
	}

	defer func() {
		if errdefs.IsNotFound(retErr) {
			retErr = nil
		}
		if retErr != nil {
			return
		}

		if err := os.RemoveAll(stateDir); err != nil {
			logrus.WithError(retErr).Warnf("failed to remove container state dir %s", stateDir)
		}
		if name != "" {
			if err := namst.Release(name, id); err != nil {
				logrus.WithError(retErr).Warnf("failed to release container Name %s", name)
			}
		}
		if err := hostsstore.DeallocHostsFile(dataStore, ns, id); err != nil {
			logrus.WithError(retErr).Warnf("failed to remove hosts file for container %q", id)
		}
	}()
	if anonVolumesJSON, ok := l[labels.AnonymousVolumes]; ok && removeAnonVolumes {
		var anonVolumes []string
		if err := json.Unmarshal([]byte(anonVolumesJSON), &anonVolumes); err != nil {
			return err
		}
		volStore, err := volume.Store(cmd)
		if err != nil {
			return err
		}
		defer func() {
			if _, err := volStore.Remove(anonVolumes); err != nil {
				logrus.WithError(err).Warnf("failed to remove anonymous volume %v", anonVolumes)
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
			return common.NewStatusError(fmt.Errorf("you cannot remove a %v container %v. Unpause the container before attempting removal or force remove", status.Status, id))
		}
		_, err := task.Delete(ctx, containerd.WithProcessKill)
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to delete task %v: %w", id, err)
		}
	// default is the case, when status.Status = containerd.Running
	default:
		if !force {
			return common.NewStatusError(fmt.Errorf("you cannot remove a %v container %v. Stop the container before attempting removal or force remove", status.Status, id))
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
	return err
}
