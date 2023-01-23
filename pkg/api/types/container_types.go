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

// KillOptions specifies options for `nerdctl (container) kill`.
type KillOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// KillSignal is the signal to send to the container
	KillSignal string
}

// ContainerRemoveOptions specifies options for `nerdctl (container) rm`.
type ContainerRemoveOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Force enables to remove a running|paused|unknown container (uses SIGKILL)
	Force bool
	// Volumes removes anonymous volumes associated with the container
	Volumes bool
}

// ContainerInspectOptions specifies options for `nerdctl container inspect`
type ContainerInspectOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Format of the output
	Format string
	// Inspect mode, either dockercompat or native
	Mode string
	// Containers are the containers to be inspected
	Containers []string
}

// ContainerCommitOptions specifies options for `nerdctl (container) commit`.
type ContainerCommitOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")
	Author string
	// Commit message
	Message string
	// Apply Dockerfile instruction to the created image (supported directives: [CMD, ENTRYPOINT])
	Change []string
	// Pause container during commit
	Pause bool
}
