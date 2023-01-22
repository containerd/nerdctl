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

// VolumeCreateCommandOptions specifies options for `nerdctl volume create`.
type VolumeCreateCommandOptions struct {
	GOptions GlobalCommandOptions
	// Labels are the volume labels
	Labels []string
}

// VolumeInspectCommandOptions specifies options for `nerdctl volume inspect`.
type VolumeInspectCommandOptions struct {
	GOptions GlobalCommandOptions
	// Format the output using the given go template
	Format string
	// Display the disk usage of volumes. Can be slow with volumes having loads of directories.
	Size bool
}

// VolumeListCommandOptions specifies options for `nerdctl volume ls`.
type VolumeListCommandOptions struct {
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

// VolumePruneCommandOptions specifies options for `nerdctl volume prune`.
type VolumePruneCommandOptions struct {
	GOptions GlobalCommandOptions
	// Do not prompt for confirmation
	Force bool
}

// VolumeRemoveCommandOptions specifies options for `nerdctl volume rm`.
type VolumeRemoveCommandOptions struct {
	GOptions GlobalCommandOptions
	// Force the removal of one or more volumes
	Force bool
}
