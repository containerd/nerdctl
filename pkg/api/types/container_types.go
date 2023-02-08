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

import (
	"io"
	"time"
)

// ContainerStartOptions specifies options for the `nerdctl (container) start`.
type ContainerStartOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Attach specifies whether to attach to the container's stdio.
	Attach bool
}

// KillOptions specifies options for `nerdctl (container) kill`.
type KillOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// KillSignal is the signal to send to the container
	KillSignal string
}

// ContainerStopOptions specifies options for `nerdctl (container) stop`.
type ContainerStopOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Timeout specifies how long to wait after sending a SIGTERM and before sending a SIGKILL.
	// If it's nil, the default is 10 seconds.
	Timeout *time.Duration
}

// ContainerRestartOptions specifies options for `nerdctl (container) restart`.
type ContainerRestartOptions struct {
	Stdout  io.Writer
	GOption GlobalCommandOptions
	// Time to wait after sending a SIGTERM and before sending a SIGKILL.
	Timeout *time.Duration
}

// ContainerPauseOptions specifies options for `nerdctl (container) pause`.
type ContainerPauseOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
}

// ContainerUnpauseOptions specifies options for `nerdctl (container) unpause`.
type ContainerUnpauseOptions ContainerPauseOptions

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

// ContainerPruneOptions specifies options for `nerdctl container prune`.
type ContainerPruneOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Force will not prompt for confirmation
	Force bool
}

// ContainerTopOptions specifies options for `nerdctl top`.
type ContainerTopOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
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

// ContainerLogsOptions specifies options for `nerdctl (container) logs`.
type ContainerLogsOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options.
	GOptions GlobalCommandOptions
	// Follow specifies whether to stream the logs or just print the existing logs.
	Follow bool
	// Timestamps specifies whether to show the timestamps of the logs.
	Timestamps bool
	// Tail specifies the number of lines to show from the end of the logs.
	// Specify 0 to show all logs.
	Tail uint
	// Show logs since timestamp (e.g., 2013-01-02T13:23:37Z) or relative (e.g., 42m for 42 minutes).
	Since string
	// Show logs before a timestamp (e.g., 2013-01-02T13:23:37Z) or relative (e.g., 42m for 42 minutes).
	Until string
}

// ContainerWaitOptions specifies options for `nerdctl (container) wait`.
type ContainerWaitOptions struct {
	Stdout io.Writer
	// GOptions is the global options.
	GOptions GlobalCommandOptions
}

// ContainerExecOptions specifies options for `nerdctl (container) exec`
type ContainerExecOptions struct {
	GOptions GlobalCommandOptions
	// Allocate a pseudo-TTY
	TTY bool
	// Keep STDIN open even if not attached
	Interactive bool
	// Detached mode: run command in the background
	Detach bool
	// Working directory inside the container
	Workdir string
	// Set environment variables
	Env []string
	// Set environment variables from file
	EnvFile []string
	// Give extended privileges to the command
	Privileged bool
	// Username or UID (format: <name|uid>[:<group|gid>])
	User string
}
