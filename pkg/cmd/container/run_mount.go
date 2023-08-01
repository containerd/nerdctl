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

package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/userns"
	"github.com/containerd/continuity/fs"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/volume"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
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

		sort.Slice(s.Mounts, func(i, j int) bool {
			// Consistent with the less function in Docker.
			// https://github.com/moby/moby/blob/0db417451313474133c5ed62bbf95e2d3c92444d/daemon/volumes.go#L34
			return strings.Count(filepath.Clean(s.Mounts[i].Destination), string(os.PathSeparator)) < strings.Count(filepath.Clean(s.Mounts[j].Destination), string(os.PathSeparator))
		})

		return nil
	}
}

// parseMountFlags parses --volume, --mount and --tmpfs.
func parseMountFlags(volStore volumestore.VolumeStore, options types.ContainerCreateOptions) ([]*mountutil.Processed, error) {
	var parsed []*mountutil.Processed //nolint:prealloc
	for _, v := range strutil.DedupeStrSlice(options.Volume) {
		x, err := mountutil.ProcessFlagV(v, volStore)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, x)
	}

	for _, v := range strutil.DedupeStrSlice(options.Tmpfs) {
		x, err := mountutil.ProcessFlagTmpfs(v)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, x)
	}

	for _, v := range strutil.DedupeStrSlice(options.Mount) {
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
func generateMountOpts(ctx context.Context, client *containerd.Client, ensuredImage *imgutil.EnsuredImage, options types.ContainerCreateOptions) ([]oci.SpecOpts, []string, []*mountutil.Processed, error) {
	// volume store is corresponds to a directory like `/var/lib/nerdctl/1935db59/volumes/default`
	volStore, err := volume.Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
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

		if err := ensuredImage.Image.Unpack(ctx, options.GOptions.Snapshotter); err != nil {
			return nil, nil, nil, fmt.Errorf("error unpacking image: %w", err)
		}

		diffIDs, err := ensuredImage.Image.RootFS(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		chainID := identity.ChainID(diffIDs).String()

		s := client.SnapshotService(options.GOptions.Snapshotter)
		tempDir, err = os.MkdirTemp("", "initialC")
		if err != nil {
			return nil, nil, nil, err
		}
		// We use Remove here instead of RemoveAll.
		// The RemoveAll will delete the temp dir and all children it contains.
		// When the Unmount fails, RemoveAll will incorrectly delete data from the mounted dir
		defer os.Remove(tempDir)

		// Add a lease of 1 hour to the view so that it is not garbage collected
		// Note(gsamfira): should we make this shorter?
		ctx, done, err := client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create lease: %w", err)
		}
		defer done(ctx)

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
		} else if runtime.GOOS == "linux" {
			defer unmounter(tempDir)
			for _, m := range mounts {
				m := m
				if m.Type == "bind" && userns.RunningInUserNS() {
					// For https://github.com/containerd/nerdctl/issues/2056
					unpriv, err := mountutil.UnprivilegedMountFlags(m.Source)
					if err != nil {
						return nil, nil, nil, err
					}
					m.Options = strutil.DedupeStrSlice(append(m.Options, unpriv...))
				}
				if err := m.Mount(tempDir); err != nil {
					if rmErr := s.Remove(ctx, tempDir); rmErr != nil && !errdefs.IsNotFound(rmErr) {
						return nil, nil, nil, rmErr
					}
					return nil, nil, nil, fmt.Errorf("failed to mount %+v on %q: %w", m, tempDir, err)
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

	if parsed, err := parseMountFlags(volStore, options); err != nil {
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

	containers, err := client.Containers(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	vfSet := strutil.SliceToSet(options.VolumesFrom)
	var vfMountPoints []dockercompat.MountPoint
	var vfAnonVolumes []string

	for _, c := range containers {
		ls, err := c.Labels(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		_, idMatch := vfSet[c.ID()]
		nameMatch := false
		if name, found := ls[labels.Name]; found {
			_, nameMatch = vfSet[name]
		}

		if idMatch || nameMatch {
			if av, found := ls[labels.AnonymousVolumes]; found {
				err = json.Unmarshal([]byte(av), &vfAnonVolumes)
				if err != nil {
					return nil, nil, nil, err
				}
			}
			if m, found := ls[labels.Mounts]; found {
				err = json.Unmarshal([]byte(m), &vfMountPoints)
				if err != nil {
					return nil, nil, nil, err
				}
			}

			ps := processeds(vfMountPoints)
			s, err := c.Spec(ctx)
			if err != nil {
				return nil, nil, nil, err
			}
			opts = append(opts, withMounts(s.Mounts))
			anonVolumes = append(anonVolumes, vfAnonVolumes...)
			mountPoints = append(mountPoints, ps...)
		}
	}

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
