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
	"strings"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

// Prune remove all stopped containers
func Prune(ctx context.Context, client *containerd.Client, options types.ContainerPruneOptions) error {
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	var deleted []string
	for _, c := range containers {
		if err = RemoveContainer(ctx, c, options.GOptions, false, true, client); err == nil {
			deleted = append(deleted, c.ID())
			continue
		}
		if errors.As(err, &ErrContainerStatus{}) {
			continue
		}
		log.G(ctx).WithError(err).Warnf("failed to remove container %s", c.ID())
	}

	if len(deleted) > 0 {
		fmt.Fprintln(options.Stdout, "Deleted Containers:")
		fmt.Fprintln(options.Stdout, strings.Join(deleted, "\n"))
	}

	return nil
}
