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
	"runtime"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/namestore"
)

// Rename change container name to a new name
// containerID is container name, short ID, or long ID
func Rename(ctx context.Context, client *containerd.Client, containerID, newContainerName string,
	options types.ContainerRenameOptions) error {
	dataStore, err := clientutil.DataStore(options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}
	namest, err := namestore.New(dataStore, options.GOptions.Namespace)
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
			return renameContainer(ctx, found.Container, newContainerName,
				options.GOptions.Namespace, namest, hostst)
		},
	}

	if n, err := walker.Walk(ctx, containerID); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", containerID)
	}
	return nil
}

func renameContainer(ctx context.Context, container containerd.Container, newName, ns string,
	namst namestore.NameStore, hostst hostsstore.Store) error {
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	name := l[labels.Name]
	if err := namst.Rename(name, container.ID(), newName); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := hostst.Update(ns, container.ID(), newName); err != nil {
			log.G(ctx).WithError(err).Warn("failed to update host networking definitions " +
				"- if your container is using network 'none', this is expected - otherwise, please report this as a bug")
		}
	}
	labels := map[string]string{
		labels.Name: newName,
	}
	if _, err = container.SetLabels(ctx, labels); err != nil {
		return err
	}
	return nil
}
