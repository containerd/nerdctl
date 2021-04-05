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
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/sys"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type Processed struct {
	Mount           specs.Mount
	AnonymousVolume string // name
}

func ProcessFlagV(s string, volStore volumestore.VolumeStore) (*Processed, error) {
	var (
		res      Processed
		src, dst string
		ro       bool
	)
	split := strings.Split(s, ":")
	switch len(split) {
	case 1:
		dst = s
		res.AnonymousVolume = idgen.GenerateID()
		logrus.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
		anonVol, err := volStore.Create(res.AnonymousVolume)
		if err != nil {
			return nil, err
		}
		src = anonVol.Mountpoint
	case 2, 3:
		src, dst = split[0], split[1]
		if !strings.Contains(src, "/") {
			// assume src is a volume name
			vol, err := volStore.Get(src)
			if err != nil {
				return nil, err
			}
			// src is now full path
			src = vol.Mountpoint
		}
		if !filepath.IsAbs(src) {
			logrus.Warnf("expected an absolute path, got a relative path %q (allowed for nerdctl, but disallowed for Docker, so unrecommended)", src)
			var err error
			src, err = filepath.Abs(src)
			if err != nil {
				return nil, err
			}
		}
		if !filepath.IsAbs(dst) {
			return nil, errors.Errorf("expected an absolute path, got %q", dst)
		}
		if len(split) == 3 {
			opts := strings.Split(split[2], ",")
			for _, opt := range opts {
				switch opt {
				case "rw":
					// NOP
				case "ro":
					ro = true
				default:
					logrus.Warnf("unsupported volume option %q", opt)
				}
			}
		}
	default:
		return nil, errors.Errorf("failed to parse %q", s)
	}
	res.Mount = specs.Mount{
		Type:        "none",
		Source:      src,
		Destination: dst,
		Options:     []string{"rbind"},
	}
	if ro {
		res.Mount.Options = append(res.Mount.Options, "ro")
	}
	if sys.RunningInUserNS() {
		unpriv, err := getUnprivilegedMountFlags(src)
		if err != nil {
			return nil, err
		}
		res.Mount.Options = strutil.DedupeStrSlice(append(res.Mount.Options, unpriv...))
	}
	return &res, nil
}

// getUnprivilegedMountFlags is from https://github.com/moby/moby/blob/v20.10.5/daemon/oci_linux.go#L420-L450
//
// Get the set of mount flags that are set on the mount that contains the given
// path and are locked by CL_UNPRIVILEGED. This is necessary to ensure that
// bind-mounting "with options" will not fail with user namespaces, due to
// kernel restrictions that require user namespace mounts to preserve
// CL_UNPRIVILEGED locked flags.
func getUnprivilegedMountFlags(path string) ([]string, error) {
	var statfs unix.Statfs_t
	if err := unix.Statfs(path, &statfs); err != nil {
		return nil, err
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
