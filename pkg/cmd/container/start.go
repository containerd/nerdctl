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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
)

// Start starts a list of `containers`. If attach is true, it only starts a single container.
func Start(ctx context.Context, client *containerd.Client, reqs []string, options types.ContainerStartOptions) error {
	if options.Attach && len(reqs) > 1 {
		return fmt.Errorf("you cannot start and attach multiple containers at once")
	}

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			var err error
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := containerutil.Start(ctx, found.Container, options.Attach, client, options.DetachKeys); err != nil {
				return err
			}
			if !options.Attach {
				_, err := fmt.Fprintln(options.Stdout, found.Req)
				if err != nil {
					return err
				}
			}
			return err
		},
	}

	return walker.WalkAll(ctx, reqs, true)
}
