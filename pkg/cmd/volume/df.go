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

package volume

import (
	"context"
	"slices"
	"strings"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

// DiskUsage reports how much disk space the local volumes of the current namespace use.
//
// A volume is active when at least one container mounts it, and the space of the volumes no
// container mounts can be reclaimed. Note that, unlike `nerdctl volume prune`, this counts the named
// volumes too: Docker reports what `docker volume prune --all` would free.
func DiskUsage(ctx context.Context, client *containerd.Client, gOptions types.GlobalCommandOptions, verbose bool) (types.VolumeDiskUsage, error) {
	du := types.VolumeDiskUsage{}

	// The size is what we are after here, so it is always requested.
	vols, err := Volumes(gOptions.Namespace, gOptions.DataRoot, gOptions.Address, true, nil)
	if err != nil {
		return du, err
	}

	containers, err := client.Containers(ctx)
	if err != nil {
		return du, err
	}
	links, err := usedVolumes(ctx, containers)
	if err != nil {
		return du, err
	}

	for _, v := range vols {
		du.TotalCount++
		du.TotalSize += v.Size
		if links[v.Name] > 0 {
			du.ActiveCount++
		} else {
			du.Reclaimable += v.Size
		}

		if verbose {
			du.Items = append(du.Items, types.VolumeDiskUsageItem{
				Name:  v.Name,
				Links: links[v.Name],
				Size:  v.Size,
			})
		}
	}

	// Volumes comes from a map, so give the verbose output a stable order.
	slices.SortFunc(du.Items, func(a, b types.VolumeDiskUsageItem) int {
		return strings.Compare(a.Name, b.Name)
	})

	return du, nil
}
