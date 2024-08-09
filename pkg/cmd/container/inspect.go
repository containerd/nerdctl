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
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/containerinspector"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
)

// Inspect prints detailed information for each container in `containers`.
func Inspect(ctx context.Context, client *containerd.Client, containers []string, options types.ContainerInspectOptions) error {
	f := &containerInspector{
		mode:        options.Mode,
		size:        options.Size,
		snapshotter: containerdutil.SnapshotService(client, options.GOptions.Snapshotter),
	}

	walker := &containerwalker.ContainerWalker{
		Client:  client,
		OnFound: f.Handler,
	}

	err := walker.WalkAll(ctx, containers, true)
	if len(f.entries) > 0 {
		if formatErr := formatter.FormatSlice(options.Format, options.Stdout, f.entries); formatErr != nil {
			log.L.Error(formatErr)
		}
	}

	return err
}

type containerInspector struct {
	mode        string
	size        bool
	snapshotter snapshots.Snapshotter
	entries     []interface{}
}

func (x *containerInspector) Handler(ctx context.Context, found containerwalker.Found) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	n, err := containerinspector.Inspect(ctx, found.Container)
	if err != nil {
		return err
	}
	switch x.mode {
	case "native":
		x.entries = append(x.entries, n)
	case "dockercompat":
		d, err := dockercompat.ContainerFromNative(n)
		if err != nil {
			return err
		}
		if x.size {
			resourceUsage, allResourceUsage, err := imgutil.ResourceUsage(ctx, x.snapshotter, d.ID)
			if err == nil {
				d.SizeRw = &resourceUsage.Size
				d.SizeRootFs = &allResourceUsage.Size
			}
		}
		x.entries = append(x.entries, d)
		return err
	default:
		return fmt.Errorf("unknown mode %q", x.mode)
	}
	return nil
}
