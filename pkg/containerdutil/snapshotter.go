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

package containerdutil

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/snapshots"
)

// SnapshotService should be called to get a new caching snapshotter
func SnapshotService(client *containerd.Client, snapshotterName string) snapshots.Snapshotter {
	return &snapshotterWithCache{
		client.SnapshotService(snapshotterName),
		map[string]snapshots.Info{},
		map[string]snapshots.Usage{},
	}
}

type snapshotterWithCache struct {
	snapshots.Snapshotter
	statCache  map[string]snapshots.Info
	usageCache map[string]snapshots.Usage
}

func (snap *snapshotterWithCache) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	if stat, ok := snap.statCache[key]; ok {
		return stat, nil
	}
	stat, err := snap.Snapshotter.Stat(ctx, key)
	if err == nil {
		snap.statCache[key] = stat
	}
	return stat, err
}

func (snap *snapshotterWithCache) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	if usage, ok := snap.usageCache[key]; ok {
		return usage, nil
	}
	usage, err := snap.Snapshotter.Usage(ctx, key)
	if err == nil {
		snap.usageCache[key] = usage
	}
	return usage, err
}
