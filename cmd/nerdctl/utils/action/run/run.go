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

package run

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/spf13/cobra"
)

func SetCreateFlags(cmd *cobra.Command) {

	// No "-h" alias for "--help", because "-h" for "--hostname".
	cmd.Flags().Bool("help", false, "show help")

	cmd.Flags().BoolP("tty", "t", false, "(Currently -t needs to correspond to -i)")
	cmd.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")
	cmd.Flags().String("restart", "no", `Restart policy to apply when a container exits (implemented values: "no"|"always")`)
	cmd.RegisterFlagCompletionFunc("restart", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"no", "always", "on-failure", "unless-stopped"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().Bool("rm", false, "Automatically remove the container when it exits")
	cmd.Flags().String("pull", "missing", `Pull image before running ("always"|"missing"|"never")`)
	cmd.RegisterFlagCompletionFunc("pull", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"always", "missing", "never"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("stop-signal", "SIGTERM", "Signal to stop a container")
	cmd.Flags().Int("stop-timeout", 0, "Timeout (in seconds) to stop a container")

	// #region for init process
	cmd.Flags().Bool("init", false, "Run an init process inside the container, Default to use tini")
	cmd.Flags().String("init-binary", common.TiniInitBinary, "The custom binary to use as the init process")
	// #endregion

	// #region platform flags
	cmd.Flags().String("platform", "", "Set platform (e.g. \"amd64\", \"arm64\")") // not a slice, and there is no --all-platforms
	cmd.RegisterFlagCompletionFunc("platform", completion.ShellCompletePlatforms)
	// #endregion

	// #region network flags
	// network (net) is defined as StringSlice, not StringArray, to allow specifying "--network=cni1,cni2"
	cmd.Flags().StringSlice("network", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none"|"container:<container>"|<CNI>)`)
	cmd.RegisterFlagCompletionFunc("network", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completion.ShellCompleteNetworkNames(cmd, []string{})
	})
	cmd.Flags().StringSlice("net", []string{netutil.DefaultNetworkName}, `Connect a container to a network ("bridge"|"host"|"none"|<CNI>)`)
	cmd.RegisterFlagCompletionFunc("net", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completion.ShellCompleteNetworkNames(cmd, []string{})
	})
	// dns is defined as StringSlice, not StringArray, to allow specifying "--dns=1.1.1.1,8.8.8.8" (compatible with Podman)
	cmd.Flags().StringSlice("dns", nil, "Set custom DNS servers")
	cmd.Flags().StringSlice("dns-search", nil, "Set custom DNS search domains")
	// We allow for both "--dns-opt" and "--dns-option", although the latter is the recommended way.
	cmd.Flags().StringSlice("dns-opt", nil, "Set DNS options")
	cmd.Flags().StringSlice("dns-option", nil, "Set DNS options")
	// publish is defined as StringSlice, not StringArray, to allow specifying "--publish=80:80,443:443" (compatible with Podman)
	cmd.Flags().StringSliceP("publish", "p", nil, "Publish a container's port(s) to the host")
	// FIXME: not support IPV6 yet
	cmd.Flags().String("ip", "", "IPv4 address to assign to the container")
	cmd.Flags().StringP("hostname", "h", "", "Container host name")
	cmd.Flags().String("mac-address", "", "MAC address to assign to the container")
	// #endregion

	cmd.Flags().String("ipc", "", `IPC namespace to use ("host"|"private")`)
	cmd.RegisterFlagCompletionFunc("ipc", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host", "private"}, cobra.ShellCompDirectiveNoFileComp
	})
	// #region cgroups, namespaces, and ulimits flags
	cmd.Flags().Float64("cpus", 0.0, "Number of CPUs")
	cmd.Flags().StringP("memory", "m", "", "Memory limit")
	cmd.Flags().String("memory-reservation", "", "Memory soft limit")
	cmd.Flags().String("memory-swap", "", "Swap limit equal to memory plus swap: '-1' to enable unlimited swap")
	cmd.Flags().Int64("memory-swappiness", -1, "Tune container memory swappiness (0 to 100) (default -1)")
	cmd.Flags().String("kernel-memory", "", "Kernel memory limit (deprecated)")
	cmd.Flags().Bool("oom-kill-disable", false, "Disable OOM Killer")
	cmd.Flags().Int("oom-score-adj", 0, "Tune container’s OOM preferences (-1000 to 1000, rootless: 100 to 1000)")
	cmd.Flags().String("pid", "", "PID namespace to use")
	cmd.RegisterFlagCompletionFunc("pid", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().Int64("pids-limit", -1, "Tune container pids limit (set -1 for unlimited)")
	cmd.Flags().StringSlice("cgroup-conf", nil, "Configure cgroup v2 (key=value)")
	cmd.Flags().Uint16("blkio-weight", 0, "Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)")
	cmd.Flags().String("cgroupns", defaults.CgroupnsMode(), `Cgroup namespace to use, the default depends on the cgroup version ("host"|"private")`)
	cmd.RegisterFlagCompletionFunc("cgroupns", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"host", "private"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("cpuset-cpus", "", "CPUs in which to allow execution (0-3, 0,1)")
	cmd.Flags().String("cpuset-mems", "", "MEMs in which to allow execution (0-3, 0,1)")
	cmd.Flags().Uint64("cpu-shares", 0, "CPU shares (relative weight)")
	cmd.Flags().Int64("cpu-quota", -1, "Limit CPU CFS (Completely Fair Scheduler) quota")
	cmd.Flags().Uint64("cpu-period", 0, "Limit CPU CFS (Completely Fair Scheduler) period")
	// device is defined as StringSlice, not StringArray, to allow specifying "--device=DEV1,DEV2" (compatible with Podman)
	cmd.Flags().StringSlice("device", nil, "Add a host device to the container")
	// ulimit is defined as StringSlice, not StringArray, to allow specifying "--ulimit=ULIMIT1,ULIMIT2" (compatible with Podman)
	cmd.Flags().StringSlice("ulimit", nil, "Ulimit options")
	cmd.Flags().String("rdt-class", "", "Name of the RDT class (or CLOS) to associate the container with")
	// #endregion

	// user flags
	cmd.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	cmd.Flags().String("umask", "", "Set the umask inside the container. Defaults to 0022")
	cmd.Flags().StringSlice("group-add", []string{}, "Add additional groups to join")

	// #region security flags
	cmd.Flags().StringArray("security-opt", []string{}, "Security options")
	cmd.RegisterFlagCompletionFunc("security-opt", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"seccomp=", "seccomp=unconfined", "apparmor=", "apparmor=" + defaults.AppArmorProfileName, "apparmor=unconfined", "no-new-privileges", "privileged-without-host-devices"}, cobra.ShellCompDirectiveNoFileComp
	})
	// cap-add and cap-drop are defined as StringSlice, not StringArray, to allow specifying "--cap-add=CAP_SYS_ADMIN,CAP_NET_ADMIN" (compatible with Podman)
	cmd.Flags().StringSlice("cap-add", []string{}, "Add Linux Capabilities")
	cmd.RegisterFlagCompletionFunc("cap-add", completion.CapShellComplete)
	cmd.Flags().StringSlice("cap-drop", []string{}, "Drop Linux Capabilities")
	cmd.RegisterFlagCompletionFunc("cap-drop", completion.CapShellComplete)
	cmd.Flags().Bool("privileged", false, "Give extended privileges to this container")
	// #endregion

	// #region runtime flags
	cmd.Flags().String("runtime", defaults.Runtime, "Runtime to use for this container, e.g. \"crun\", or \"io.containerd.runsc.v1\"")
	// sysctl needs to be StringArray, not StringSlice, to prevent "foo=foo1,foo2" from being split to {"foo=foo1", "foo2"}
	cmd.Flags().StringArray("sysctl", nil, "Sysctl options")
	// gpus needs to be StringArray, not StringSlice, to prevent "Capabilities=utility,device=DEV" from being split to {"Capabilities=utility", "device=DEV"}
	cmd.Flags().StringArray("gpus", nil, "GPU devices to Add to the container ('all' to pass all GPUs)")
	cmd.RegisterFlagCompletionFunc("gpus", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"all"}, cobra.ShellCompDirectiveNoFileComp
	})
	// #endregion

	// #region mount flags
	// volume needs to be StringArray, not StringSlice, to prevent "/foo:/foo:ro,Z" from being split to {"/foo:/foo:ro", "Z"}
	cmd.Flags().StringArrayP("volume", "v", nil, "Bind mount a volume")
	// tmpfs needs to be StringArray, not StringSlice, to prevent "/foo:size=64m,exec" from being split to {"/foo:size=64m", "exec"}
	cmd.Flags().StringArray("tmpfs", nil, "Mount a tmpfs directory")
	cmd.Flags().StringArray("mount", nil, "Attach a filesystem mount to the container")
	// #endregion

	// rootfs flags
	cmd.Flags().Bool("read-only", false, "Mount the container's root filesystem as read only")
	// rootfs flags (from Podman)
	cmd.Flags().Bool("rootfs", false, "The first argument is not an image but the rootfs to the exploded container")

	// #region env flags
	// entrypoint needs to be StringArray, not StringSlice, to prevent "FOO=foo1,foo2" from being split to {"FOO=foo1", "foo2"}
	// entrypoint StringArray is an internal implementation to support `nerdctl compose` entrypoint yaml filed with multiple strings
	// users are not expected to specify multiple --entrypoint flags manually.
	cmd.Flags().StringArray("entrypoint", nil, "Overwrite the default ENTRYPOINT of the image")
	cmd.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	// env needs to be StringArray, not StringSlice, to prevent "FOO=foo1,foo2" from being split to {"FOO=foo1", "foo2"}
	cmd.Flags().StringArrayP("env", "e", nil, "Set environment variables")
	// add-host is defined as StringSlice, not StringArray, to allow specifying "--add-host=HOST1:IP1,HOST2:IP2" (compatible with Podman)
	cmd.Flags().StringSlice("add-host", nil, "Add a custom host-to-IP mapping (host:ip)")
	// env-file is defined as StringSlice, not StringArray, to allow specifying "--env-file=FILE1,FILE2" (compatible with Podman)
	cmd.Flags().StringSlice("env-file", nil, "Set environment variables from file")

	// #region metadata flags
	cmd.Flags().String("name", "", "Assign a name to the container")
	// label needs to be StringArray, not StringSlice, to prevent "foo=foo1,foo2" from being split to {"foo=foo1", "foo2"}
	cmd.Flags().StringArrayP("label", "l", nil, "Set metadata on container")
	cmd.RegisterFlagCompletionFunc("label", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return labels.ShellCompletions, cobra.ShellCompDirectiveNoFileComp
	})

	// label-file is defined as StringSlice, not StringArray, to allow specifying "--env-file=FILE1,FILE2" (compatible with Podman)
	cmd.Flags().StringSlice("label-file", nil, "Set metadata on container from file")
	cmd.Flags().String("cidfile", "", "Write the container ID to the file")
	// #endregion

	// #region logging flags
	// log-opt needs to be StringArray, not StringSlice, to prevent "env=os,customer" from being split to {"env=os", "customer"}
	cmd.Flags().String("log-driver", "json-file", "Logging driver for the container. Default is json-file. It also supports logURI (eg: --log-driver binary://<path>)")
	cmd.RegisterFlagCompletionFunc("log-driver", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return logging.Drivers(), cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringArray("log-opt", nil, "Log driver options")
	// #endregion

	// shared memory flags
	cmd.Flags().String("shm-size", "", "Size of /dev/shm")
	cmd.Flags().String("pidfile", "", "file path to write the task's pid")

	// #region verify flags
	cmd.Flags().String("verify", "none", "Verify the image (none|cosign)")
	cmd.RegisterFlagCompletionFunc("verify", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().String("cosign-key", "", "Path to the public key file, KMS, URI or Kubernetes Secret for --verify=cosign")
	// #endregion
}

// WithBindMountHostIPC replaces /dev/shm and /dev/mqueue  mount with rbind.
// Required for --ipc=host on rootless.
func WithBindMountHostIPC(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		if path.Clean(m.Destination) == "/dev/shm" {
			newM := specs.Mount{
				Destination: "/dev/shm",
				Type:        "bind",
				Source:      "/dev/shm",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
		if path.Clean(m.Destination) == "/dev/mqueue" {
			newM := specs.Mount{
				Destination: "/dev/mqueue",
				Type:        "bind",
				Source:      "/dev/mqueue",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
	}
	return nil
}

// withBindMountHostProcfs replaces procfs mount with rbind.
// Required for --pid=host on rootless.
//
// https://github.com/moby/moby/pull/41893/files
// https://github.com/containers/podman/blob/v3.0.0-rc1/pkg/specgen/generate/oci.go#L248-L257
func WithBindMountHostProcfs(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		if path.Clean(m.Destination) == "/proc" {
			newM := specs.Mount{
				Destination: "/proc",
				Type:        "bind",
				Source:      "/proc",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
	}

	// Remove ReadonlyPaths for /proc/*
	newROP := s.Linux.ReadonlyPaths[:0]
	for _, x := range s.Linux.ReadonlyPaths {
		x = path.Clean(x)
		if !strings.HasPrefix(x, "/proc/") {
			newROP = append(newROP, x)
		}
	}
	s.Linux.ReadonlyPaths = newROP
	return nil
}

func GenerateSharingPIDOpts(ctx context.Context, targetCon containerd.Container) ([]oci.SpecOpts, error) {
	opts := make([]oci.SpecOpts, 0)

	task, err := targetCon.Task(ctx, nil)
	if err != nil {
		return nil, err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return nil, err
	}

	if status.Status != containerd.Running {
		return nil, fmt.Errorf("shared container is not running")
	}

	spec, err := targetCon.Spec(ctx)
	if err != nil {
		return nil, err
	}

	isHost := true
	for _, n := range spec.Linux.Namespaces {
		if n.Type == specs.PIDNamespace {
			isHost = false
		}
	}
	if isHost {
		opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
		if rootlessutil.IsRootless() {
			opts = append(opts, WithBindMountHostProcfs)
		}
	} else {
		ns := specs.LinuxNamespace{
			Type: specs.PIDNamespace,
			Path: fmt.Sprintf("/proc/%d/ns/pid", task.Pid()),
		}
		opts = append(opts, oci.WithLinuxNamespace(ns))
	}

	return opts, nil
}
