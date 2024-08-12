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
	"os"
	"runtime"
	"syscall"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/volume"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/ipcutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/namestore"
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
func Remove(ctx context.Context, client *containerd.Client, containers []string, options types.ContainerRemoveOptions) error {
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := RemoveContainer(ctx, found.Container, options.GOptions, options.Force, options.Volumes, client); err != nil {
				if errors.As(err, &ErrContainerStatus{}) {
					err = fmt.Errorf("%s. unpause/stop container first or force removal", err)
				}
				return err
			}
			_, err := fmt.Fprintln(options.Stdout, found.Req)
			return err
		},
	}

	err := walker.WalkAll(ctx, containers, true)
	if err != nil && options.Force {
		log.G(ctx).Error(err)
		return nil
	}
	return err
}

// RemoveContainer removes a container from containerd store.
// It will first retrieve system objects (namestore, etcetera), then assess whether we should remove the container or not
// based of "force" and the status of the task.
// If we are to delete, it then kills and delete the task.
// If task removal fails, we stop (except if it was just "NotFound").
// We then enter the defer cleanup function that will:
// - remove the network config (windows only)
// - delete the container
// - then and ONLY then, on a successful container remove, clean things-up on our side (volume store, etcetera)
// If you do need to add more cleanup, please do so at the bottom of the defer function
func RemoveContainer(ctx context.Context, c containerd.Container, globalOptions types.GlobalCommandOptions, force bool, removeAnonVolumes bool, client *containerd.Client) (retErr error) {
	// defer the storage of remove error in the dedicated label
	defer func() {
		if retErr != nil {
			containerutil.UpdateErrorLabel(ctx, c, retErr)
		}
	}()

	// Get namespace
	containerNamespace, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return err
	}
	// Get labels
	containerLabels, err := c.Labels(ctx)
	if err != nil {
		return err
	}
	// Get datastore
	dataStore, err := clientutil.DataStore(globalOptions.DataRoot, globalOptions.Address)
	if err != nil {
		return err
	}
	// Get namestore
	nameStore, err := namestore.New(dataStore, containerNamespace)
	if err != nil {
		return err
	}
	// Get volume store
	volStore, err := volume.Store(globalOptions.Namespace, globalOptions.DataRoot, globalOptions.Address)
	if err != nil {
		return err
	}
	// Note: technically, it is not strictly necessary to acquire an exclusive lock on the volume store here.
	// Worst case scenario, we would fail removing anonymous volumes later on, which is a soft error, and which would
	// only happen if we concurrently tried to remove the same container.
	err = volStore.Lock()
	if err != nil {
		return err
	}
	defer volStore.Unlock()
	// Decode IPC
	ipc, err := ipcutil.DecodeIPCLabel(containerLabels[labels.IPC])
	if err != nil {
		return err
	}

	// Get the container id, stateDir and name
	id := c.ID()
	stateDir := containerLabels[labels.StateDir]
	name := containerLabels[labels.Name]

	// This will evaluate retErr to decide if we proceed with removal or not
	defer func() {
		// If there was an error, and it was not "NotFound", this is a hard error, we stop here and do nothing.
		if retErr != nil && !errdefs.IsNotFound(retErr) {
			return
		}

		// Otherwise, nil the error so that we do not write the error label on the container
		retErr = nil

		// Now, delete the actual container
		var delOpts []containerd.DeleteOpts
		if _, err := c.Image(ctx); err == nil {
			delOpts = append(delOpts, containerd.WithSnapshotCleanup)
		}

		// NOTE: on non-Windows platforms, network cleanup is performed by OCI hooks.
		// Seeing as though Windows does not currently support OCI hooks, we must explicitly
		// perform the network cleanup from the main nerdctl executable.
		if runtime.GOOS == "windows" {
			spec, err := c.Spec(ctx)
			if err != nil {
				retErr = err
				return
			}

			netOpts, err := containerutil.NetworkOptionsFromSpec(spec)
			if err != nil {
				retErr = fmt.Errorf("failed to load container networking options from specs: %s", err)
				return
			}

			networkManager, err := containerutil.NewNetworkingOptionsManager(globalOptions, netOpts, client)
			if err != nil {
				retErr = fmt.Errorf("failed to instantiate network options manager: %s", err)
				return
			}

			if err := networkManager.CleanupNetworking(ctx, c); err != nil {
				log.G(ctx).WithError(err).Warnf("failed to clean up container networking: %q", id)
			}
		}

		// Delete the container now. If it fails, try again without snapshot cleanup
		// If it still fails, time to stop.
		if c.Delete(ctx, delOpts...) != nil {
			retErr = c.Delete(ctx)
			if retErr != nil {
				return
			}
		}

		// Container has been removed successfully. Now we just finish the cleanup on our side.

		// Cleanup IPC - soft failure
		if err = ipcutil.CleanUp(ipc); err != nil {
			log.G(ctx).WithError(err).Warnf("failed to cleanup IPC for container %q", id)
		}

		// Remove state dir - soft failure
		if err = os.RemoveAll(stateDir); err != nil {
			log.G(ctx).WithError(err).Warnf("failed to remove container state dir %s", stateDir)
		}

		// Enforce release name here in case the poststop hook name release fails - soft failure
		if name != "" {
			if err = nameStore.Release(name, id); err != nil {
				log.G(ctx).WithError(err).Warnf("failed to release container name %s", name)
			}
		}

		// De-allocate hosts file - soft failure
		if err = hostsstore.DeallocHostsFile(dataStore, containerNamespace, id); err != nil {
			log.G(ctx).WithError(err).Warnf("failed to remove hosts file for container %q", id)
		}

		// Volume removal is not handled by the poststop hook lifecycle because it depends on removeAnonVolumes option
		if anonVolumesJSON, ok := containerLabels[labels.AnonymousVolumes]; ok && removeAnonVolumes {
			var anonVolumes []string
			if err = json.Unmarshal([]byte(anonVolumesJSON), &anonVolumes); err != nil {
				log.G(ctx).WithError(err).Warnf("failed to unmarshall anonvolume information for container %q", id)
			} else {
				var errs []error
				_, errs, err = volStore.Remove(anonVolumes)
				if err != nil || len(errs) > 0 {
					log.G(ctx).WithError(err).Warnf("failed to remove anonymous volumes %v", anonVolumes)
				}
			}
		}
	}()

	// Get the task.
	task, err := c.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	// Task was here, get the status
	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	// Now, we have a live task with a status.
	switch status.Status {
	case containerd.Paused:
		// Paused containers only get removed if we force
		if !force {
			return NewStatusError(id, status.Status)
		}
	case containerd.Running:
		// Running containers only get removed if we force
		if !force {
			return NewStatusError(id, status.Status)
		}
		// Kill the task. Soft error.
		if err = task.Kill(ctx, syscall.SIGKILL); err != nil && !errdefs.IsNotFound(err) {
			log.G(ctx).WithError(err).Warnf("failed to send SIGKILL to task %v", id)
		}
		es, err := task.Wait(ctx)
		if err == nil {
			<-es
		}
	case containerd.Created:
		// TODO(Iceber): Since `containerd.WithProcessKill` blocks the killing of tasks with PID 0,
		// remove the judgment and break when it is compatible with the tasks.
		if task.Pid() == 0 {
			// Created tasks with PID 0 always get removed
			// Delete the task, without forcing kill
			_, err = task.Delete(ctx)
			return err
		}
	case containerd.Stopped:
		// Stopped containers always get removed
		// Delete the task, without forcing kill
		_, err = task.Delete(ctx)
		return err
	default:
		// Unknown status error out
		return fmt.Errorf("unknown container status %s", status.Status)
	}

	// Delete the task
	_, err = task.Delete(ctx, containerd.WithProcessKill)
	return err
}
