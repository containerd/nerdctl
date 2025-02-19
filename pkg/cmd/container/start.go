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
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/labels"
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

			var attachStreams []string
			// Only get attach streams if we're dealing with a single container request
			if len(reqs) == 1 {
				attachStreams, err = getAttachStreams(ctx, found.Container, options)
				if err != nil {
					return err
				}
			}
			if err := containerutil.Start(ctx, found.Container, attachStreams, client, options.DetachKeys); err != nil {
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

func getAttachStreams(ctx context.Context, container containerd.Container, options types.ContainerStartOptions) ([]string, error) {
	if options.Attach {
		// If start is called with --attach option, --attach attaches to STDOUT and STDERR
		// source: https://github.com/containerd/nerdctl/blob/main/docs/command-reference.md#whale-nerdctl-start
		//
		return []string{"STDOUT", "STDERR"}, nil
	}

	// Otherwise, check if attach streams were set in container labels during container create
	ctrlabels, err := container.Labels(ctx)
	if err != nil {
		return []string{}, err
	}

	attachJSON, ok := ctrlabels[labels.AttachStreams]
	if !ok {
		return []string{}, nil
	}

	var attachStreams []string
	if err := json.Unmarshal([]byte(attachJSON), &attachStreams); err != nil {
		return []string{}, err
	}

	if len(attachStreams) == 0 {
		return []string{}, nil
	}

	for _, stream := range attachStreams {
		if stream != "STDOUT" && stream != "STDERR" && stream != "STDIN" {
			return []string{}, fmt.Errorf("invalid attach stream in labels: %s", stream)
		}
	}

	return attachStreams, nil
}
