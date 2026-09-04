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

package types

import "time"

// DiskUsage is the disk usage of a single containerd namespace, as reported by `nerdctl system df`.
// The build cache is not namespaced by containerd; it is scoped by the BuildKit host instead.
type DiskUsage struct {
	Images     ImageDiskUsage
	Containers ContainerDiskUsage
	Volumes    VolumeDiskUsage
	BuildCache BuildCacheDiskUsage
}

// ImageDiskUsage is the disk usage of the images of a namespace.
//
// TotalSize is the deduplicated total: every snapshot and every content blob is counted once, even
// when it is shared by several images. It is therefore not the sum of the Size of Items.
type ImageDiskUsage struct {
	TotalCount  int64
	ActiveCount int64
	TotalSize   int64
	Reclaimable int64
	// Items is only populated when the verbose output was requested
	Items []ImageDiskUsageItem
}

// ImageDiskUsageItem is the disk usage of a single image.
type ImageDiskUsageItem struct {
	ID         string
	Repository string
	Tag        string
	CreatedAt  time.Time
	// Size is the content present in the content store plus the unpacked snapshots
	Size int64
	// SharedSize is the part of Size that is also used by at least one other image
	SharedSize int64
	// Containers is the number of containers created from this image
	Containers int64
}

// ContainerDiskUsage is the disk usage of the containers of a namespace.
type ContainerDiskUsage struct {
	TotalCount  int64
	ActiveCount int64
	TotalSize   int64
	Reclaimable int64
	// Items is only populated when the verbose output was requested
	Items []ContainerDiskUsageItem
}

// ContainerDiskUsageItem is the disk usage of a single container.
type ContainerDiskUsageItem struct {
	ID           string
	Image        string
	Command      string
	LocalVolumes int64
	// SizeRw is the size of the read-write layer, without the size of the image
	SizeRw    int64
	CreatedAt time.Time
	Status    string
	Names     string
}

// VolumeDiskUsage is the disk usage of the local volumes of a namespace.
type VolumeDiskUsage struct {
	TotalCount  int64
	ActiveCount int64
	TotalSize   int64
	Reclaimable int64
	// Items is only populated when the verbose output was requested
	Items []VolumeDiskUsageItem
}

// VolumeDiskUsageItem is the disk usage of a single volume.
type VolumeDiskUsageItem struct {
	Name string
	// Links is the number of containers referencing this volume
	Links int64
	Size  int64
}

// BuildCacheDiskUsage is the disk usage of the BuildKit build cache.
type BuildCacheDiskUsage struct {
	TotalCount  int64
	ActiveCount int64
	TotalSize   int64
	Reclaimable int64
	// Items is only populated when the verbose output was requested
	Items []BuildCacheDiskUsageItem
}

// BuildCacheDiskUsageItem is the disk usage of a single build cache record.
type BuildCacheDiskUsageItem struct {
	ID         string
	CacheType  string
	Size       int64
	CreatedAt  time.Time
	LastUsedAt *time.Time
	UsageCount int
	InUse      bool
	Shared     bool
}
