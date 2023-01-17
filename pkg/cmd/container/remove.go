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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/volume"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/sirupsen/logrus"
)

var _ error = ErrContainerStatus{}

// ErrContainerStatus represents an error that container is in a status unexpected
// by the caller. E.g., remove a non-stoped/non-created container without force.
type ErrContainerStatus struct {
	ID     string
	Status containerd.ProcessStatus
}

func (e ErrContainerStatus) Error() string {
	return fmt.Sprintf("container %s is in %v status", e.ID, e.Status)
}

// NewStatusError creates an ErrContainerStatus from container id and status.
func NewStatusError(id string, status containerd.ProcessStatus) error {
	return ErrContainerStatus{
		ID:     id,
		Status: status,
	}
}

// Remove removes a list of `containers`.
func Remove(ctx context.Context, containers []string, opt types.ContainerRemoveCommandOptions, out io.Writer) error {
	client, ctx, cancel, err := clientutil.NewClient(ctx, opt.GOptions.Namespace, opt.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := RemoveContainer(ctx, found.Container, opt.GOptions, opt.Force, opt.Volumes); err != nil {
				if errors.As(err, &ErrContainerStatus{}) {
					err = fmt.Errorf("%s. unpause/stop container first or force removal", err)
				}
				return err
			}
			_, err = fmt.Fprintf(out, "%s\n", found.Req)
			return err
		},
	}
	for _, req := range containers {
		n, err := walker.Walk(ctx, req)
		if err == nil && n == 0 {
			err = fmt.Errorf("no such container %s", req)
		}
		if err != nil {
			if opt.Force {
				logrus.Error(err)
			} else {
				return err
			}
		}
	}
	return nil
}

// RemoveContainer removes a container from containerd store.
func RemoveContainer(ctx context.Context, c containerd.Container, globalOptions types.GlobalCommandOptions, force bool, removeAnonVolumes bool) (retErr error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return err
	}
	id := c.ID()
	l, err := c.Labels(ctx)
	if err != nil {
		return err
	}
	stateDir := l[labels.StateDir]
	name := l[labels.Name]
	dataStore, err := clientutil.DataStore(globalOptions.DataRoot, globalOptions.Address)
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
				logrus.WithError(retErr).Warnf("failed to release container name %s", name)
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
		volStore, err := volume.Store(globalOptions.Namespace, globalOptions.DataRoot, globalOptions.Address)
		if err != nil {
			return err
		}
		defer func() {
			if _, err := volStore.Remove(anonVolumes); err != nil {
				logrus.WithError(err).Warnf("failed to remove anonymous volumes %v", anonVolumes)
			}
		}()
	}

	task, err := c.Task(ctx, cio.Load)
	if err != nil {
		if errdefs.IsNotFound(err) {
			if c.Delete(ctx, containerd.WithSnapshotCleanup) != nil {
				return c.Delete(ctx)
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
			return NewStatusError(id, status.Status)
		}
		_, err := task.Delete(ctx, containerd.WithProcessKill)
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to delete task %v: %w", id, err)
		}
	// default is the case, when status.Status = containerd.Running
	default:
		if !force {
			return NewStatusError(id, status.Status)
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
	if _, err := c.Image(ctx); err == nil {
		delOpts = append(delOpts, containerd.WithSnapshotCleanup)
	}

	if err := c.Delete(ctx, delOpts...); err != nil {
		return err
	}
	return err
}
