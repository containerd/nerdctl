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

import "io"

// VolumeCreateOptions specifies options for `nerdctl volume create`.
type VolumeCreateOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Labels are the volume labels
	Labels []string
}

// VolumeInspectOptions specifies options for `nerdctl volume inspect`.
type VolumeInspectOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Format the output using the given go template
	Format string
	// Display the disk usage of volumes. Can be slow with volumes having loads of directories.
	Size bool
}

// VolumeListOptions specifies options for `nerdctl volume ls`.
type VolumeListOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Only display volume names
	Quiet bool
	// Format the output using the given go template
	Format string
	// Display the disk usage of volumes. Can be slow with volumes having loads of directories.
	Size bool
	// Filter matches volumes based on given conditions
	Filters []string
}

// VolumePruneOptions specifies options for `nerdctl volume prune`.
type VolumePruneOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	//Remove all unused volumes, not just anonymous ones
	All bool
	// Do not prompt for confirmation
	Force bool
}

// VolumeRemoveOptions specifies options for `nerdctl volume rm`.
type VolumeRemoveOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Force the removal of one or more volumes
	Force bool
}
