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
	// The key sequence for detaching a container.
	DetachKeys string
}

// ContainerKillOptions specifies options for `nerdctl (container) kill`.
type ContainerKillOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// KillSignal is the signal to send to the container
	KillSignal string
}

// ContainerCreateOptions specifies options for `nerdctl (container) create` and `nerdctl (container) run`.
type ContainerCreateOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions

	// NerdctlCmd is the command name of nerdctl
	NerdctlCmd string
	// NerdctlArgs is the arguments of nerdctl
	NerdctlArgs []string

	// InRun is true when it's generated in the `run` command
	InRun bool

	// #region for basic flags
	// Interactive keep STDIN open even if not attached
	Interactive bool
	// TTY specifies whether to allocate a pseudo-TTY for the container
	TTY bool
	// Detach runs container in background and print container ID
	Detach bool
	// The key sequence for detaching a container.
	DetachKeys string
	// Restart specifies the policy to apply when a container exits
	Restart string
	// Rm specifies whether to remove the container automatically when it exits
	Rm bool
	// Pull image before running, default is missing
	Pull string
	// Pid namespace to use
	Pid string
	// StopSignal signal to stop a container, default is SIGTERM
	StopSignal string
	// StopTimeout specifies the timeout (in seconds) to stop a container
	StopTimeout int
	// #endregion

	// #region for platform flags
	// Platform set target platform for build (e.g., "amd64", "arm64", "windows", "freebsd")
	Platform string
	// #endregion

	// #region for init process flags
	// InitProcessFlag specifies to run an init inside the container that forwards signals and reaps processes
	InitProcessFlag bool
	// InitBinary specifies the custom init binary to use, default is tini
	InitBinary *string
	// #endregion

	// #region for isolation flags
	// Isolation specifies the container isolation technology
	Isolation string
	// #endregion

	// #region for resource flags
	// CPUs specifies the number of CPUs
	CPUs float64
	// CPUQuota limits the CPU CFS (Completely Fair Scheduler) quota
	CPUQuota int64
	// CPUPeriod limits the CPU CFS (Completely Fair Scheduler) period
	CPUPeriod uint64
	// CPUShares specifies the CPU shares (relative weight)
	CPUShares uint64
	// CPUSetCPUs specifies the CPUs in which to allow execution (0-3, 0,1)
	CPUSetCPUs string
	// CPUSetMems specifies the memory nodes (MEMs) in which to allow execution (0-3, 0,1). Only effective on NUMA systems.
	CPUSetMems string
	// Memory specifies the memory limit
	Memory string
	// MemoryReservationChanged specifies whether the memory soft limit has been changed
	MemoryReservationChanged bool
	// MemoryReservation specifies the memory soft limit
	MemoryReservation string
	// MemorySwap specifies the swap limit equal to memory plus swap: '-1' to enable unlimited swap
	MemorySwap string
	// MemSwappinessChanged specifies whether the memory swappiness has been changed
	MemorySwappiness64Changed bool
	// MemorySwappiness64 specifies the tune container memory swappiness (0 to 100) (default -1)
	MemorySwappiness64 int64
	// KernelMemoryChanged specifies whether the kernel memory limit has been changed
	KernelMemoryChanged bool
	// KernelMemory specifies the kernel memory limit(deprecated)
	KernelMemory string
	// OomKillDisable specifies whether to disable OOM Killer
	OomKillDisable bool
	// OomScoreAdjChanged specifies whether the OOM preferences has been changed
	OomScoreAdjChanged bool
	// OomScoreAdj specifies the tune container’s OOM preferences (-1000 to 1000, rootless: 100 to 1000)
	OomScoreAdj int
	// PidsLimit specifies the tune container pids limit
	PidsLimit int64
	// CgroupConf specifies to configure cgroup v2 (key=value)
	CgroupConf []string
	// BlkioWeight specifies the block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)
	BlkioWeight uint16
	// Cgroupns specifies the cgroup namespace to use
	Cgroupns string
	// CgroupParent specifies the optional parent cgroup for the container
	CgroupParent string
	// Device specifies add a host device to the container
	Device []string
	// #endregion

	// #region for intel RDT flags
	// RDTClass specifies the Intel Resource Director Technology (RDT) class
	RDTClass string
	// #endregion

	// #region for user flags
	// User specifies the user to run the container as
	User string
	// Umask specifies the umask to use for the container
	Umask string
	// GroupAdd specifies additional groups to join
	GroupAdd []string
	// #endregion

	// #region for security flags
	// SecurityOpt specifies security options
	SecurityOpt []string
	// CapAdd add Linux capabilities
	CapAdd []string
	// CapDrop drop Linux capabilities
	CapDrop []string
	// Privileged gives extended privileges to this container
	Privileged bool
	// #endregion

	// #region for runtime flags
	// Runtime to use for this container, e.g. "crun", or "io.containerd.runsc.v1".
	Runtime string
	// Sysctl set sysctl options, e.g "net.ipv4.ip_forward=1"
	Sysctl []string
	// #endregion

	// #region for volume flags
	// Volume specifies a list of volumes to mount
	Volume []string
	// Tmpfs specifies a list of tmpfs mounts
	Tmpfs []string
	// Mount specifies a list of mounts to mount
	Mount []string
	// #endregion

	// #region for rootfs flags
	// ReadOnly mount the container's root filesystem as read only
	ReadOnly bool
	// Rootfs specifies the first argument is not an image but the rootfs to the exploded container. Corresponds to Podman CLI.
	Rootfs bool
	// #endregion

	// #region for env flags
	// EntrypointChanged specifies whether the entrypoint has been changed
	EntrypointChanged bool
	// Entrypoint overwrites the default ENTRYPOINT of the image
	Entrypoint []string
	// Workdir set the working directory for the container
	Workdir string
	// Env set environment variables
	Env []string
	// EnvFile set environment variables from file
	EnvFile []string
	// #endregion

	// #region for metadata flags
	// NameChanged specifies whether the name has been changed
	NameChanged bool
	// Name assign a name to the container
	Name string
	// Label set meta data on a container
	Label []string
	// LabelFile read in a line delimited file of labels
	LabelFile []string
	// CidFile write the container ID to the file
	CidFile string
	// PidFile specifies the file path to write the task's pid. The CLI syntax conforms to Podman convention.
	PidFile string
	// #endregion

	// #region for logging flags
	// LogDriver set the logging driver for the container
	LogDriver string
	// LogOpt set logging driver specific options
	LogOpt []string
	// #endregion

	// #region for shared memory flags
	// IPC namespace to use
	IPC string
	// ShmSize set the size of /dev/shm
	ShmSize string
	// #endregion

	// #region for gpu flags
	// GPUs specifies GPU devices to add to the container ('all' to pass all GPUs). Please see also ./gpu.md for details.
	GPUs []string
	// #endregion

	// #region for ulimit flags
	// Ulimit set ulimits
	Ulimit []string
	// #endregion

	// #region for ipfs flags
	// IPFSAddress specifies the multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)
	IPFSAddress string
	// #endregion

	// ImagePullOpt specifies image pull options which holds the ImageVerifyOptions for verifying the image.
	ImagePullOpt ImagePullOptions
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

// ContainerPruneOptions specifies options for `nerdctl (container) prune`.
type ContainerPruneOptions struct {
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

// ContainerRenameOptions specifies options for `nerdctl (container) rename`.
type ContainerRenameOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
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

// ContainerListOptions specifies options for `nerdctl (container) list`.
type ContainerListOptions struct {
	// GOptions is the global options.
	GOptions GlobalCommandOptions
	// Show all containers (default shows just running).
	All bool
	// Show n last created containers (includes all states). Non-positive values are ignored.
	// In other words, if LastN is positive, All will be set to true.
	LastN int
	// Truncate output (e.g., container ID, command of the container main process, etc.) or not.
	Truncate bool
	// Display total file sizes.
	Size bool
	// Filters matches containers based on given conditions.
	Filters []string
}

// ContainerCpOptions specifies options for `nerdctl (container) cp`
type ContainerCpOptions struct {
	// GOptions is the global options.
	GOptions       GlobalCommandOptions
	Container2Host bool
	ContainerReq   string
	// Destination path to copy file to.
	DestPath string
	// Source path to copy file from.
	SrcPath string
	// Follow symbolic links in SRC_PATH
	FollowSymLink bool
}

// ContainerStatsOptions specifies options for `nerdctl stats`.
type ContainerStatsOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options.
	GOptions GlobalCommandOptions
	// Show all containers (default shows just running).
	All bool
	// Pretty-print images using a Go template, e.g., {{json .}}.
	Format string
	// Disable streaming stats and only pull the first result.
	NoStream bool
	// Do not truncate output.
	NoTrunc bool
}
