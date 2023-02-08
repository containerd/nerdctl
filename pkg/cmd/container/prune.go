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
	"errors"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/sirupsen/logrus"
)

// Prune will remove all stopped containers.
func Prune(ctx context.Context, client *containerd.Client, options types.ContainerPruneOptions) error {
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	var deleted []string
	for _, c := range containers {
		err = RemoveContainer(ctx, c, options.GOptions, false, true)
		if err == nil {
			deleted = append(deleted, c.ID())
			continue
		}
		if errors.As(err, &ErrContainerStatus{}) {
			continue
		}
		logrus.WithError(err).Warnf("failed to remove container %s", c.ID())
	}

	if len(deleted) > 0 {
		fmt.Fprintln(options.Stdout, "Deleted Containers:")
		for _, id := range deleted {
			fmt.Fprintln(options.Stdout, id)
		}
		fmt.Fprintln(options.Stdout, "")
	}

	return nil
}
