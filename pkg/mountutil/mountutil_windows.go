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
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/sirupsen/logrus"
	"path/filepath"
	"strings"
)

func getUnprivilegedMountFlags(path string) ([]string, error) {
	m := []string{}
	return m, nil
}

// DefaultPropagationMode is the default propagation of mounts
// where user doesn't specify mount propagation explicitly.
// See also: https://github.com/moby/moby/blob/v20.10.7/volume/mounts/windows_parser.go#L440-L442
const DefaultPropagationMode = ""

// parseVolumeOptions parses specified optsRaw with using information of
// the volume type and the src directory when necessary.
//func parseVolumeOptions(vType, src, optsRaw string) ([]string, []oci.SpecOpts, error) {
//	var writeModeRawOpts []string
//	for _, opt := range strings.Split(optsRaw, ",") {
//		switch opt {
//		case "rw":
//			writeModeRawOpts = append(writeModeRawOpts, opt)
//		case "ro":
//			writeModeRawOpts = append(writeModeRawOpts, opt)
//		case "":
//			// NOP
//		default:
//			logrus.Warnf("unsupported volume option %q", opt)
//		}
//	}
//	var opts []string
//	if len(writeModeRawOpts) > 1 {
//		return nil, nil, fmt.Errorf("duplicated read/write volume option: %+v", writeModeRawOpts)
//	} else if len(writeModeRawOpts) > 0 && writeModeRawOpts[0] == "ro" {
//		opts = append(opts, "ro")
//	} // No need to return option when "rw"
//	return opts, nil, nil
//}
//

func ProcessFlagTmpfs(s string) (*Processed, error) {
	return nil, errdefs.ErrNotImplemented
}

func parseVolumeOptions(vType, src, optsRaw string) ([]string, []oci.SpecOpts, error) {
	return parseVolumeOptionsWithMountInfo(vType, src, optsRaw, getMountInfo)
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
func parseVolumeOptionsWithMountInfo(vType, src, optsRaw string, getMountInfoFunc func(string) (mount.Info, error)) ([]string, []oci.SpecOpts, error) {
	var (
		writeModeRawOpts   []string
		propagationRawOpts []string
	)
	for _, opt := range strings.Split(optsRaw, ",") {
		switch opt {
		case "rw", "ro", "rro":
			writeModeRawOpts = append(writeModeRawOpts, opt)
		case "private", "rprivate", "shared", "rshared", "slave", "rslave":
			propagationRawOpts = append(propagationRawOpts, opt)
		case "":
			// NOP
		default:
			logrus.Warnf("unsupported volume option %q", opt)
		}
	}

	var opts []string
	var specOpts []oci.SpecOpts

	if len(writeModeRawOpts) > 1 {
		return nil, nil, fmt.Errorf("duplicated read/write volume option: %+v", writeModeRawOpts)
	} else if len(writeModeRawOpts) > 0 {
		switch writeModeRawOpts[0] {
		case "ro":
			opts = append(opts, "ro")
		case "rro":
			// Mount option "rro" is supported since crun v1.4 / runc v1.1 (https://github.com/opencontainers/runc/pull/3272), with kernel >= 5.12.
			// Older version of runc just ignores "rro", so we have to add "ro" too, to our best effort.
			opts = append(opts, "ro", "rro")
			if len(propagationRawOpts) != 1 || propagationRawOpts[0] != "rprivate" {
				logrus.Warn("Mount option \"rro\" should be used in conjunction with \"rprivate\"")
			}
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

func ProcessFlagMount(s string, volStore volumestore.VolumeStore) (*Processed, error) {
	return nil, errdefs.ErrNotImplemented
}
