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

package mountutil

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	mobymount "github.com/moby/sys/mount"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/v2/pkg/ociruntimeutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

/*
   Portions from https://github.com/moby/moby/blob/v20.10.5/daemon/oci_linux.go
   Portions from https://github.com/moby/moby/blob/v20.10.5/volume/mounts/linux_parser.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/v20.10.5/NOTICE
*/

const (
	DefaultMountType = "none"

	// DefaultPropagationMode is the default propagation of mounts
	// where user doesn't specify mount propagation explicitly.
	// See also: https://github.com/moby/moby/blob/v20.10.7/volume/mounts/linux_parser.go#L145
	DefaultPropagationMode = "rprivate"
)

// UnprivilegedMountFlags is from https://github.com/moby/moby/blob/v20.10.5/daemon/oci_linux.go#L420-L450
//
// Get the set of mount flags that are set on the mount that contains the given
// path and are locked by CL_UNPRIVILEGED. This is necessary to ensure that
// bind-mounting "with options" will not fail with user namespaces, due to
// kernel restrictions that require user namespace mounts to preserve
// CL_UNPRIVILEGED locked flags.
func UnprivilegedMountFlags(path string) ([]string, error) {
	var statfs unix.Statfs_t
	if err := unix.Statfs(path, &statfs); err != nil {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: err}
	}

	// The set of keys come from https://github.com/torvalds/linux/blob/v4.13/fs/namespace.c#L1034-L1048.
	unprivilegedFlags := map[uint64]string{
		unix.MS_RDONLY:     "ro",
		unix.MS_NODEV:      "nodev",
		unix.MS_NOEXEC:     "noexec",
		unix.MS_NOSUID:     "nosuid",
		unix.MS_NOATIME:    "noatime",
		unix.MS_RELATIME:   "relatime",
		unix.MS_NODIRATIME: "nodiratime",
	}

	var flags []string
	for mask, flag := range unprivilegedFlags {
		if uint64(statfs.Flags)&mask == mask {
			flags = append(flags, flag)
		}
	}

	return flags, nil
}

// supportsRecursivelyReadOnly is replaced in unit tests.
var supportsRecursivelyReadOnly = ociruntimeutil.SupportsRecursivelyReadOnly

// readOnlyMode is the read-only mode of a mount.
// The modes correspond to the BindOptions of the Docker API >= v1.44.
// https://github.com/moby/moby/pull/45278
type readOnlyMode int

const (
	// readOnlyModeRecursiveIfPossible makes the mount recursively read-only when
	// the kernel and the OCI runtime support the "rro" mount option, and falls
	// back to the plain (non-recursive) read-only otherwise.
	// This is the default mode of read-only mounts since Docker v25.
	readOnlyModeRecursiveIfPossible readOnlyMode = iota
	// readOnlyModeNonRecursive makes the mount read-only, but keeps its submounts
	// writable. This was the default mode of read-only mounts until Docker v24.
	// Corresponds to `--mount type=bind,readonly,bind-recursive=writable`.
	readOnlyModeNonRecursive
	// readOnlyModeForceRecursive makes the mount recursively read-only, or
	// raises an error when the kernel or the OCI runtime does not support "rro".
	// Corresponds to `--mount type=bind,readonly,bind-recursive=readonly`.
	readOnlyModeForceRecursive
)

// readOnlyMountOptions returns the mount options for the given read-only mode.
// Whether the OCI runtime supports the "rro" mount option is detected by running
// `$RUNTIME features`; ociRuntime is the value of the `--runtime` flag.
func readOnlyMountOptions(mode readOnlyMode, ociRuntime string) ([]string, error) {
	switch mode {
	case readOnlyModeRecursiveIfPossible:
		if err := supportsRecursivelyReadOnly(ociRuntime); err != nil {
			log.L.WithError(err).Debug("recursive read-only mounts are not supported, falling back to non-recursive read-only")
			return []string{"ro"}, nil
		}
		return []string{"rro"}, nil
	case readOnlyModeNonRecursive:
		return []string{"ro"}, nil
	case readOnlyModeForceRecursive:
		if err := supportsRecursivelyReadOnly(ociRuntime); err != nil {
			return nil, err
		}
		return []string{"rro"}, nil
	}
	return nil, fmt.Errorf("unexpected read-only mode %v", mode)
}

// parseVolumeOptions parses specified optsRaw with using information of
// the volume type and the src directory when necessary.
func parseVolumeOptions(vType, src, optsRaw, ociRuntime string) ([]string, []oci.SpecOpts, error) {
	return parseVolumeOptionsWithMountInfo(vType, src, optsRaw, ociRuntime, getMountInfo)
}

// getMountInfo gets mount.Info of a directory.
func getMountInfo(dir string) (mount.Info, error) {
	sourcePath, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return mount.Info{}, err
	}
	return mount.Lookup(sourcePath)
}

// parseVolumeOptionsWithMountInfo is the testable implementation
// of parseVolumeOptions.
func parseVolumeOptionsWithMountInfo(vType, src, optsRaw, ociRuntime string, getMountInfoFunc func(string) (mount.Info, error)) ([]string, []oci.SpecOpts, error) {
	var (
		writeModeRawOpts   []string
		propagationRawOpts []string
		bindOpts           []string
	)
	var specOpts []oci.SpecOpts
	for _, opt := range strings.Split(optsRaw, ",") {
		switch opt {
		case "rw", "ro", "rro":
			writeModeRawOpts = append(writeModeRawOpts, opt)
		case "private", "rprivate", "shared", "rshared", "slave", "rslave":
			propagationRawOpts = append(propagationRawOpts, opt)
		case "bind", "rbind":
			// bind means not recursively bind-mounted, rbind is the opposite
			bindOpts = append(bindOpts, opt)
		case "Z", "z":
			specOpts = append(specOpts, func(ctx context.Context, cli oci.Client, c *containers.Container, s *oci.Spec) error {
				if s.Linux != nil && s.Linux.MountLabel != "" {
					if err := label.Relabel(src, s.Linux.MountLabel, opt == "z"); err != nil {
						return err
					}
				}
				return nil
			})
		case "":
			// NOP
		default:
			log.L.Warnf("unsupported volume option %q", opt)
		}
	}

	var opts []string

	if len(bindOpts) > 0 && vType != Bind {
		return nil, nil, fmt.Errorf("volume bind/rbind option is only supported for bind mount: %+v", bindOpts)
	} else if len(bindOpts) > 1 {
		return nil, nil, fmt.Errorf("duplicated bind/rbind option: %+v", bindOpts)
	} else if len(bindOpts) > 0 {
		opts = append(opts, bindOpts[0])
	}

	if len(writeModeRawOpts) > 1 {
		return nil, nil, fmt.Errorf("duplicated read/write volume option: %+v", writeModeRawOpts)
	} else if len(writeModeRawOpts) > 0 {
		switch writeModeRawOpts[0] {
		case "ro":
			// Docker (since v25) attempts to make the mount recursively read-only.
			// https://github.com/moby/moby/pull/45278
			roOpts, err := readOnlyMountOptions(readOnlyModeRecursiveIfPossible, ociRuntime)
			if err != nil {
				return nil, nil, err
			}
			opts = append(opts, roOpts...)
		case "rro":
			// "rro" was introduced in nerdctl v0.14 (2021), ahead of Docker.
			// Docker v25 introduced `--mount type=bind,...,readonly,bind-recursive=readonly` instead.
			log.L.Warn("The volume option \"rro\" is deprecated; use `--mount type=bind,src=...,dst=...,readonly,bind-propagation=rprivate,bind-recursive=readonly` instead")
			if len(propagationRawOpts) != 1 || propagationRawOpts[0] != "rprivate" {
				log.L.Warn("Mount option \"rro\" should be used in conjunction with \"rprivate\"")
			}
			roOpts, err := readOnlyMountOptions(readOnlyModeForceRecursive, ociRuntime)
			if err != nil {
				return nil, nil, err
			}
			opts = append(opts, roOpts...)
		case "rw":
			// NOP
		default:
			// NOTREACHED
			return nil, nil, fmt.Errorf("unexpected writeModeRawOpts[0]=%q", writeModeRawOpts[0])
		}
	}

	if len(propagationRawOpts) > 1 {
		return nil, nil, fmt.Errorf("duplicated volume propagation option: %+v", propagationRawOpts)
	} else if len(propagationRawOpts) > 0 && vType != Bind {
		return nil, nil, fmt.Errorf("volume propagation option is only supported for bind mount: %+v", propagationRawOpts)
	} else if vType == Bind {
		var pFlag string
		var got string
		if len(propagationRawOpts) > 0 {
			got = propagationRawOpts[0]
		}
		switch got {
		case "shared", "rshared":
			pFlag = got
			// a bind mount can be shared from shared mount
			mi, err := getMountInfoFunc(src)
			if err != nil {
				return nil, nil, err
			}
			if err := ensureMountOptionalValue(mi, "shared:"); err != nil {
				return nil, nil, err
			}

			// NOTE: Though OCI Runtime Spec doesn't explicitly describe, runc's default
			//       of RootfsPropagation is unix.MS_SLAVE | unix.MS_REC (i.e. runc applies
			//       "slave" to all mount points in the container recursively). This ends
			//       up marking the bind src directories "slave" and preventing it to shared
			//      with the host. So we set RootfsPropagation to "shared" here.
			//
			// See also:
			// - OCI Runtime Spec: https://github.com/opencontainers/runtime-spec/blob/v1.0.2/config-linux.md#rootfs-mount-propagation
			// - runc implementation: https://github.com/opencontainers/runc/blob/v1.0.0/libcontainer/rootfs_linux.go#L771-L777
			specOpts = append(specOpts, func(ctx context.Context, cli oci.Client, c *containers.Container, s *oci.Spec) error {
				switch s.Linux.RootfsPropagation {
				case "shared", "rshared":
					// NOP
				default:
					s.Linux.RootfsPropagation = "shared"
				}
				return nil
			})
		case "slave", "rslave":
			pFlag = got
			// a bind mount can be a slave of shared or an existing slave mount
			mi, err := getMountInfoFunc(src)
			if err != nil {
				return nil, nil, err
			}
			if err := ensureMountOptionalValue(mi, "shared:", "master:"); err != nil {
				return nil, nil, err
			}

			// See above comments about RootfsPropagation. Here we make sure that
			// the mountpoint can be a slave of the host mount.
			specOpts = append(specOpts, func(ctx context.Context, cli oci.Client, c *containers.Container, s *oci.Spec) error {
				switch s.Linux.RootfsPropagation {
				case "shared", "rshared", "slave", "rslave":
					// NOP
				default:
					s.Linux.RootfsPropagation = "rslave"
				}
				return nil
			})
		case "private", "rprivate":
			pFlag = got
		default:
			// No propagation is specified to this bind mount.
			// NOTE: When RootfsPropagation is set (e.g. by other bind mount option), that
			//       propagation mode will be applied to this bind mount as well. So we need
			//       to set "rprivate" explicitly for preventing this bind mount from unexpectedly
			//       shared with the host. This behaviour is compatible to docker:
			//       https://github.com/moby/moby/blob/v20.10.7/volume/mounts/linux_parser.go#L320-L322
			//
			// TODO: directories managed by containerd (e.g. /var/lib/containerd, /run/containerd, ...)
			//       should be marked as "rslave" instead of "rprivate". This is because allowing
			//       containers to hold their private bind mounts will prevent containerd from remove
			//       them. See also: https://github.com/moby/moby/pull/36055.
			//       Unfortunately, containerd doesn't expose the locations of directories where it manages.
			//       Current workaround is explicitly add "rshared" or "rslave" option to these bind mounts.
			pFlag = DefaultPropagationMode
		}
		opts = append(opts, pFlag)
	}

	return opts, specOpts, nil
}

// ensure the mount of the specified directory has either of the specified
// "optional" value in the entry in the /proc/<pid>/mountinfo file.
//
// For more details about "optional" field:
// - https://github.com/moby/sys/blob/mountinfo/v0.4.1/mountinfo/mountinfo.go#L52-L56
func ensureMountOptionalValue(mi mount.Info, vals ...string) error {
	var hasValue bool
	for _, opt := range strings.Split(mi.Optional, " ") {
		for _, mark := range vals {
			if strings.HasPrefix(opt, mark) {
				hasValue = true
			}
		}
	}
	if !hasValue {
		return fmt.Errorf("mountpoint %q doesn't have optional field neither of %+v", mi.Mountpoint, vals)
	}
	return nil
}

func ProcessFlagTmpfs(s string) (*Processed, error) {
	split := strings.SplitN(s, ":", 2)
	dst := split[0]
	options := []string{"noexec", "nosuid", "nodev"}
	if len(split) == 2 {
		raw := append(options, strings.Split(split[1], ",")...)
		var err error
		options, err = mobymount.MergeTmpfsOptions(raw)
		if err != nil {
			return nil, err
		}
	}
	res := &Processed{
		Mount: specs.Mount{
			Type:        "tmpfs",
			Source:      "tmpfs",
			Destination: dst,
			Options:     options,
		},
		Type: Tmpfs,
		Mode: strings.Join(options, ","),
	}
	return res, nil
}

func ProcessFlagMount(s string, volStore volumestore.VolumeStore, ociRuntime string) (*Processed, error) {
	fields := strings.Split(s, ",")
	var (
		mountType        string
		src              string
		dst              string
		bindPropagation  string
		bindNonRecursive bool
		bindRecursive    string // "enabled", "disabled", "writable", or "readonly"
		rwOption         string
		tmpfsSize        int64
		tmpfsMode        os.FileMode
		err              error
	)

	// set default values
	mountType = Volume
	tmpfsMode = os.FileMode(01777)

	// three types of mount(and examples):
	// --mount type=bind,source="$(pwd)"/target,target=/app2,readonly,bind-propagation=shared
	// --mount type=tmpfs,destination=/app,tmpfs-mode=1770,tmpfs-size=1MB
	// --mount type=volume,src=vol-1,dst=/app,readonly
	// if type not specified, default will be set to volume
	// --mount src=`pwd`/tmp,target=/app

	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		key := strings.ToLower(parts[0])

		if len(parts) == 1 {
			switch key {
			case "readonly", "ro":
				rwOption = key
				continue
			case "rro":
				log.L.Warn("The mount option \"rro\" is deprecated; use \"readonly\" with \"bind-propagation=rprivate\" and \"bind-recursive=readonly\" instead")
				rwOption = key
				continue
			case "bind-nonrecursive":
				// Removed in Docker v29, in favor of `bind-recursive=disabled` https://github.com/docker/cli/pull/6241
				log.L.Warn("The mount option \"bind-nonrecursive\" is deprecated; use \"bind-recursive=disabled\" instead")
				bindNonRecursive = true
				continue
			}
		}

		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid field '%s' must be a key=value pair", field)
		}

		value := parts[1]
		switch key {
		case "type":
			switch value {
			case "tmpfs":
				mountType = Tmpfs
			case "bind":
				mountType = Bind
			case "volume":
			default:
				return nil, fmt.Errorf("invalid mount type '%s' must be a volume/bind/tmpfs", value)
			}
		case "source", "src":
			src = value
		case "target", "dst", "destination":
			dst = value
		case "readonly", "ro", "rro":
			trueValue, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid value for %s: %s", key, value)
			}
			if key == "rro" {
				log.L.Warn("The mount option \"rro\" is deprecated; use \"readonly\" with \"bind-propagation=rprivate\" and \"bind-recursive=readonly\" instead")
			}
			if trueValue {
				rwOption = key
			}
		case "bind-propagation":
			// here don't validate the propagation value
			// parseVolumeOptions will do that.
			bindPropagation = value
		case "bind-nonrecursive":
			// Removed in Docker v29, in favor of `bind-recursive=disabled` https://github.com/docker/cli/pull/6241
			log.L.Warn("The mount option \"bind-nonrecursive\" is deprecated; use \"bind-recursive=disabled\" instead")
			bindNonRecursive, err = strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid value for %s: %s", key, value)
			}
		case "bind-recursive":
			// bind-recursive is the Docker (v25) option that supersedes bind-nonrecursive.
			// https://github.com/docker/cli/pull/4316
			switch value {
			case "enabled", "disabled", "writable", "readonly":
				bindRecursive = value
			default:
				return nil, fmt.Errorf("invalid value for %s: %s (must be \"enabled\", \"disabled\", \"writable\", or \"readonly\")", key, value)
			}
		case "tmpfs-size":
			tmpfsSize, err = units.RAMInBytes(value)
			if err != nil {
				return nil, fmt.Errorf("invalid value for %s: %s", key, value)
			}
		case "tmpfs-mode":
			ui64, err := strconv.ParseUint(value, 8, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid value for %s: %s", key, value)
			}
			tmpfsMode = os.FileMode(ui64)
		default:
			return nil, fmt.Errorf("unexpected key '%s' in '%s'", key, field)
		}
	}

	// Resolve the read-only mode of the mount, for Docker (v25) compatibility.
	// https://github.com/docker/cli/pull/4316
	roMode := readOnlyModeRecursiveIfPossible
	if rwOption == "rro" {
		// Deprecated form: force RRO, like `bind-recursive=readonly` (but the
		// propagation is not validated, for compatibility with older nerdctl).
		roMode = readOnlyModeForceRecursive
		if bindPropagation != "rprivate" {
			log.L.Warn("Mount option \"rro\" should be used in conjunction with \"bind-propagation=rprivate\"")
		}
	}
	if bindRecursive != "" {
		if mountType != Bind {
			return nil, fmt.Errorf("the option bind-recursive is only supported for bind mounts")
		}
		switch bindRecursive {
		case "enabled":
			bindNonRecursive = false
		case "disabled":
			bindNonRecursive = true
		case "writable":
			if rwOption == "" {
				return nil, fmt.Errorf("the option 'bind-recursive=writable' requires 'readonly' to be specified in conjunction")
			}
			roMode = readOnlyModeNonRecursive
		case "readonly":
			if rwOption == "" {
				return nil, fmt.Errorf("the option 'bind-recursive=readonly' requires 'readonly' to be specified in conjunction")
			}
			if bindPropagation != "rprivate" {
				return nil, fmt.Errorf("the option 'bind-recursive=readonly' requires 'bind-propagation=rprivate' to be specified in conjunction")
			}
			if bindNonRecursive {
				return nil, fmt.Errorf("the option 'bind-recursive=readonly' conflicts with 'bind-nonrecursive'")
			}
			roMode = readOnlyModeForceRecursive
		}
	}

	// compose new fileds and join into a string
	// to call legacy ProcessFlagTmpfs or ProcessFlagV function
	fields = []string{}
	options := []string{}

	switch mountType {
	case Tmpfs:
		fields = []string{dst}
		if rwOption != "" {
			options = append(options, "ro")
		}
		if tmpfsMode != 0 {
			options = append(options, fmt.Sprintf("mode=%o", tmpfsMode))
		}
		if tmpfsSize > 0 {
			options = append(options, getTmpfsSize(tmpfsSize))
		}
	case Volume, Bind:
		// The read-only option is not composed here; it is applied to the
		// processed mount below, as the legacy volume option syntax cannot
		// express all the read-only modes.
		fields = []string{src, dst}
		if bindPropagation != "" {
			options = append(options, bindPropagation)
		}
		if mountType == Bind {
			if bindNonRecursive {
				options = append(options, "bind")
			} else {
				options = append(options, "rbind")
			}
		}
	}

	if len(options) > 0 {
		optionsStr := strings.Join(options, ",")
		fields = append(fields, optionsStr)
	}
	fieldsStr := strings.Join(fields, ":")

	log.L.Debugf("Call legacy %s process, spec: %s ", mountType, fieldsStr)

	switch mountType {
	case Tmpfs:
		return ProcessFlagTmpfs(fieldsStr)
	case Volume, Bind:
		// createDir=false for --mount option to disallow creating directories on host if not found
		res, err := ProcessFlagV(fieldsStr, volStore, false, ociRuntime)
		if err != nil {
			return nil, err
		}
		if rwOption != "" {
			roOpts, err := readOnlyMountOptions(roMode, ociRuntime)
			if err != nil {
				return nil, err
			}
			res.Mount.Options = strutil.DedupeStrSlice(append(res.Mount.Options, roOpts...))
			res.Mode = strings.Join(res.Mount.Options, ",")
		}
		return res, nil
	}
	return nil, fmt.Errorf("invalid mount type '%s' must be a volume/bind/tmpfs", mountType)
}

// copy from https://github.com/moby/moby/blob/085c6a98d54720e70b28354ccec6da9b1b9e7fcf/volume/mounts/linux_parser.go#L375
func getTmpfsSize(size int64) string {
	// calculate suffix here, making this linux specific, but that is
	// okay, since API is that way anyways.

	// we do this by finding the suffix that divides evenly into the
	// value, returning the value itself, with no suffix, if it fails.
	//
	// For the most part, we don't enforce any semantic to this values.
	// The operating system will usually align this and enforce minimum
	// and maximums.
	var (
		suffix string
	)
	for _, r := range []struct {
		suffix  string
		divisor int64
	}{
		{"g", 1 << 30},
		{"m", 1 << 20},
		{"k", 1 << 10},
	} {
		if size%r.divisor == 0 {
			size = size / r.divisor
			suffix = r.suffix
			break
		}
	}

	return fmt.Sprintf("size=%d%s", size, suffix)
}
