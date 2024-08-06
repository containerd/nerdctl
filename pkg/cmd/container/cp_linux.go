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
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
)

// Cp copies files/folders between a running container and the local filesystem.
func Cp(ctx context.Context, client *containerd.Client, options types.ContainerCpOptions) error {
	foundMatchCount := 0
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			foundMatchCount = found.MatchCount
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			return containerutil.CopyFiles(
				ctx,
				client,
				found.Container,
				options.Container2Host,
				options.DestPath,
				options.SrcPath,
				options.GOptions.Snapshotter,
				options.FollowSymLink)
		},
	}
	count, err := walker.Walk(ctx, options.ContainerReq)

	if count < 1 {
		if foundMatchCount == 0 {
			err = fmt.Errorf("could not find container: %s, with error: %w", options.ContainerReq, err)
		} else {
			err = fmt.Errorf("could not find %s in container %s", options.SrcPath, options.ContainerReq)
		}
	}

	return err
}
