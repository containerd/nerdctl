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

package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/continuity/fs"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// copy from https://github.com/containerd/containerd/blob/v1.6.0-rc.1/pkg/cri/opts/spec_linux.go#L129-L151
func withMounts(mounts []specs.Mount) oci.SpecOpts {
	return func(ctx context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		// Copy all mounts from default mounts, except for
		// - mounts overridden by supplied mount;
		// - all mounts under /dev if a supplied /dev is present.
		mountSet := make(map[string]struct{})
		for _, m := range mounts {
			mountSet[filepath.Clean(m.Destination)] = struct{}{}
		}

		defaultMounts := s.Mounts
		s.Mounts = nil

		for _, m := range defaultMounts {
			dst := filepath.Clean(m.Destination)
			if _, ok := mountSet[dst]; ok {
				// filter out mount overridden by a supplied mount
				continue
			}
			if _, mountDev := mountSet["/dev"]; mountDev && strings.HasPrefix(dst, "/dev/") {
				// filter out everything under /dev if /dev is a supplied mount
				continue
			}
			s.Mounts = append(s.Mounts, m)
		}

		s.Mounts = append(s.Mounts, mounts...)
		return nil
	}
}

// parseMountFlags parses --volume, --mount and --tmpfs.
func parseMountFlags(cmd *cobra.Command, volStore volumestore.VolumeStore) ([]*mountutil.Processed, error) {
	var parsed []*mountutil.Processed //nolint:prealloc
	flagVSlice, err := cmd.Flags().GetStringArray("volume")
	if err != nil {
		return nil, err
	}
	for _, v := range strutil.DedupeStrSlice(flagVSlice) {
		x, err := mountutil.ProcessFlagV(v, volStore)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, x)
	}

	// tmpfs needs to be StringArray, not StringSlice, to prevent "/foo:size=64m,exec" from being split to {"/foo:size=64m", "exec"}
	tmpfsSlice, err := cmd.Flags().GetStringArray("tmpfs")
	if err != nil {
		return nil, err
	}
	for _, v := range strutil.DedupeStrSlice(tmpfsSlice) {
		x, err := mountutil.ProcessFlagTmpfs(v)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, x)
	}

	mountsSlice, err := cmd.Flags().GetStringArray("mount")
	if err != nil {
		return nil, err
	}
	for _, v := range strutil.DedupeStrSlice(mountsSlice) {
		x, err := mountutil.ProcessFlagMount(v, volStore)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, x)
	}

	return parsed, nil
}

// generateMountOpts generates volume-related mount opts.
// Other mounts such as procfs mount are not handled here.
func GenerateMountOpts(ctx context.Context, cmd *cobra.Command, client *containerd.Client, ensuredImage *imgutil.EnsuredImage) ([]oci.SpecOpts, []string, []*mountutil.Processed, error) {
	volStore, err := volume.Store(cmd)
	if err != nil {
		return nil, nil, nil, err
	}

	//nolint:golint,prealloc
	var (
		opts        []oci.SpecOpts
		anonVolumes []string
		userMounts  []specs.Mount
		mountPoints []*mountutil.Processed
	)
	mounted := make(map[string]struct{})
	var imageVolumes map[string]struct{}
	var tempDir string

	if ensuredImage != nil {
		imageVolumes = ensuredImage.ImageConfig.Volumes

		snapshotter, err := cmd.Flags().GetString("snapshotter")
		if err != nil {
			return nil, nil, nil, err
		}

		if err := ensuredImage.Image.Unpack(ctx, snapshotter); err != nil {
			return nil, nil, nil, fmt.Errorf("error unpacking image: %w", err)
		}

		diffIDs, err := ensuredImage.Image.RootFS(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		chainID := identity.ChainID(diffIDs).String()

		s := client.SnapshotService(snapshotter)
		tempDir, err = os.MkdirTemp("", "initialC")
		if err != nil {
			return nil, nil, nil, err
		}
		// We use Remove here instead of RemoveAll.
		// The RemoveAll will delete the temp dir and all children it contains.
		// When the Unmount fails, RemoveAll will incorrectly delete data from the mounted dir
		defer os.Remove(tempDir)

		var mounts []mount.Mount
		mounts, err = s.View(ctx, tempDir, chainID)
		if err != nil {
			return nil, nil, nil, err
		}

		// windows has additional steps for mounting see
		// https://github.com/containerd/containerd/commit/791e175c79930a34cfbb2048fbcaa8493fd2c86b
		unmounter := func(mountPath string) {
			if uerr := mount.Unmount(mountPath, 0); uerr != nil {
				logrus.Debugf("Failed to unmount snapshot %q", tempDir)
				if err == nil {
					err = uerr
				}
			}
		}

		if runtime.GOOS == "windows" {
			for _, m := range mounts {
				defer unmounter(m.Source)
				// appending the layerID to the root.
				mountPath := filepath.Join(tempDir, filepath.Base(m.Source))
				if err := m.Mount(mountPath); err != nil {
					if err := s.Remove(ctx, tempDir); err != nil && !errdefs.IsNotFound(err) {
						return nil, nil, nil, err
					}
					return nil, nil, nil, err
				}
			}
		} else {
			defer unmounter(tempDir)
			if err := mount.All(mounts, tempDir); err != nil {
				if err := s.Remove(ctx, tempDir); err != nil && !errdefs.IsNotFound(err) {
					return nil, nil, nil, err
				}
				return nil, nil, nil, err
			}
		}
	}

	if parsed, err := parseMountFlags(cmd, volStore); err != nil {
		return nil, nil, nil, err
	} else if len(parsed) > 0 {
		ociMounts := make([]specs.Mount, len(parsed))
		for i, x := range parsed {
			ociMounts[i] = x.Mount
			mounted[filepath.Clean(x.Mount.Destination)] = struct{}{}

			target, err := securejoin.SecureJoin(tempDir, x.Mount.Destination)
			if err != nil {
				return nil, nil, nil, err
			}

			// Copying content in AnonymousVolume and namedVolume
			if x.Type == "volume" {
				if err := copyExistingContents(target, x.Mount.Source); err != nil {
					return nil, nil, nil, err
				}
			}
			if x.AnonymousVolume != "" {
				anonVolumes = append(anonVolumes, x.AnonymousVolume)
			}
			opts = append(opts, x.Opts...)
		}
		userMounts = append(userMounts, ociMounts...)

		// add parsed user specified bind-mounts/volume/tmpfs to mountPoints
		mountPoints = append(mountPoints, parsed...)
	}

	// imageVolumes are defined in Dockerfile "VOLUME" instruction
	for imgVolRaw := range imageVolumes {
		imgVol := filepath.Clean(imgVolRaw)
		switch imgVol {
		case "/", "/dev", "/sys", "proc":
			return nil, nil, nil, fmt.Errorf("invalid VOLUME: %q", imgVolRaw)
		}
		if _, ok := mounted[imgVol]; ok {
			continue
		}
		anonVolName := idgen.GenerateID()

		logrus.Debugf("creating anonymous volume %q, for \"VOLUME %s\"",
			anonVolName, imgVolRaw)
		anonVol, err := volStore.Create(anonVolName, []string{})
		if err != nil {
			return nil, nil, nil, err
		}

		target, err := securejoin.SecureJoin(tempDir, imgVol)
		if err != nil {
			return nil, nil, nil, err
		}

		//copying up initial contents of the mount point directory
		if err := copyExistingContents(target, anonVol.Mountpoint); err != nil {
			return nil, nil, nil, err
		}

		m := specs.Mount{
			Type:        "none",
			Source:      anonVol.Mountpoint,
			Destination: imgVol,
			Options:     []string{"rbind"},
		}
		userMounts = append(userMounts, m)
		anonVolumes = append(anonVolumes, anonVolName)

		mountPoint := &mountutil.Processed{
			Type:            "volume",
			AnonymousVolume: anonVolName,
			Mount:           m,
		}
		mountPoints = append(mountPoints, mountPoint)
	}

	opts = append(opts, withMounts(userMounts))
	return opts, anonVolumes, mountPoints, nil
}

// copyExistingContents copies from the source to the destination and
// ensures the ownership is appropriately set.
func copyExistingContents(source, destination string) error {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	}
	dstList, err := os.ReadDir(destination)
	if err != nil {
		return err
	}
	if len(dstList) != 0 {
		logrus.Debugf("volume at %q is not initially empty, skipping copying", destination)
		return nil
	}
	return fs.CopyDir(destination, source)
}
