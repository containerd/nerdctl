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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/moby/sys/userns"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/continuity/fs"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/idgen"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/mountutil"
	"github.com/containerd/nerdctl/v2/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
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
		// createDir=true for -v option to allow creation of directory on host if not found.
		x, err := mountutil.ProcessFlagV(v, volStore, true)
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

// gcRootLabel marks a snapshot as a GC root so containerd does not reclaim it.
const gcRootLabel = "containerd.io/gc.root"

// setupImageMount ensures and unpacks ref, then creates a read-only GC-rooted
// snapshot view of its rootfs. Without a subpath it returns the snapshotter's
// own mount for destination (runc owns its lifecycle). With a subpath it
// materializes the view on a host directory and returns a read-only bind mount
// of the resolved subdirectory, because an OCI overlay mount cannot select a
// subdir; that host directory is returned so it can be unmounted on removal.
// It returns the OCI mount, the view's snapshot key, and the host materialization
// path (empty when no subpath is used).
func setupImageMount(ctx context.Context, client *containerd.Client, options types.ContainerCreateOptions, ref, destination, subpath string) (specs.Mount, string, string, error) {
	ensured, err := imgutil.EnsureImage(ctx, client, ref, options.ImagePullOpt)
	if err != nil {
		return specs.Mount{}, "", "", fmt.Errorf("failed to ensure image %q for image mount: %w", ref, err)
	}
	if err := ensured.Image.Unpack(ctx, options.GOptions.Snapshotter); err != nil {
		return specs.Mount{}, "", "", fmt.Errorf("failed to unpack image %q for image mount: %w", ref, err)
	}
	diffIDs, err := ensured.Image.RootFS(ctx)
	if err != nil {
		return specs.Mount{}, "", "", fmt.Errorf("failed to get rootfs of image %q for image mount: %w", ref, err)
	}
	chainID := identity.ChainID(diffIDs).String()

	snapshotKey := idgen.GenerateID() + "-image-mount"
	s := client.SnapshotService(options.GOptions.Snapshotter)
	mounts, err := s.View(ctx, snapshotKey, chainID, snapshots.WithLabels(map[string]string{
		gcRootLabel: time.Now().UTC().Format(time.RFC3339),
	}))
	if err != nil {
		return specs.Mount{}, "", "", fmt.Errorf("failed to create read-only view of image %q: %w", ref, err)
	}
	// removeView drops the snapshot view on any failure after it was created.
	removeView := func() {
		if rmErr := s.Remove(ctx, snapshotKey); rmErr != nil && !errdefs.IsNotFound(rmErr) {
			log.G(ctx).WithError(rmErr).Warnf("failed to remove image-mount snapshot %q", snapshotKey)
		}
	}

	if subpath != "" {
		return setupImageSubpathMount(ctx, options, ref, destination, subpath, snapshotKey, mounts, removeView)
	}

	// Whole-rootfs case: hand the snapshotter's mount straight to the OCI runtime,
	// which mounts and unmounts it with the container. overlayfs and native
	// snapshotters each yield exactly one mount for a view.
	if len(mounts) != 1 {
		removeView()
		return specs.Mount{}, "", "", fmt.Errorf("image mount expects exactly one mount from the snapshotter, got %d", len(mounts))
	}
	m := mounts[0]
	opts := m.Options
	// A view without an upper dir is already read-only; make it explicit for
	// bind-backed snapshotters.
	if !strutil.InStringSlice(opts, "ro") {
		opts = append(opts, "ro")
	}
	return specs.Mount{
		Type:        m.Type,
		Source:      m.Source,
		Destination: destination,
		Options:     opts,
	}, snapshotKey, "", nil
}

// setupImageSubpathMount materializes the snapshot view on a host directory and
// returns a read-only bind mount of the subpath. securejoin resolves the subpath
// against the materialized rootfs, applying a second check beyond parse-time
// validation: it blocks symlinks inside the image that point outside the rootfs.
// On any failure it unwinds the host mount, the directory, and the view. The
// returned host path must be unmounted and removed when the container is deleted.
func setupImageSubpathMount(ctx context.Context, options types.ContainerCreateOptions, ref, destination, subpath, snapshotKey string, mounts []mount.Mount, removeView func()) (specs.Mount, string, string, error) {
	// Materialize under the data root keyed by snapshot key so it is unique per
	// view and outlives container restarts (only removed on container deletion).
	hostMountpoint := filepath.Join(options.GOptions.DataRoot, "image-mounts", snapshotKey)
	if err := os.MkdirAll(hostMountpoint, 0o700); err != nil {
		removeView()
		return specs.Mount{}, "", "", fmt.Errorf("failed to create image-mount host dir: %w", err)
	}
	if err := mount.All(mounts, hostMountpoint); err != nil {
		// mount.All may have applied some mounts before failing; unmount before
		// removing the dir so RemoveAll never recurses into a live mount.
		if uErr := mount.UnmountAll(hostMountpoint, 0); uErr != nil {
			log.G(ctx).WithError(uErr).Warnf("failed to unmount image-mount host path %q after failed setup", hostMountpoint)
		}
		os.RemoveAll(hostMountpoint)
		removeView()
		return specs.Mount{}, "", "", fmt.Errorf("failed to materialize image %q for subpath mount: %w", ref, err)
	}
	// cleanup unwinds the host mount, its directory, and the view together.
	cleanup := func() {
		if uErr := mount.UnmountAll(hostMountpoint, 0); uErr != nil {
			log.G(ctx).WithError(uErr).Warnf("failed to unmount image-mount host path %q", hostMountpoint)
		}
		os.RemoveAll(hostMountpoint)
		removeView()
	}

	// securejoin resolves the subpath within the materialized rootfs, blocking
	// symlink escapes that parse-time validation cannot see.
	resolved, err := securejoin.SecureJoin(hostMountpoint, subpath)
	if err != nil {
		cleanup()
		return specs.Mount{}, "", "", fmt.Errorf("failed to resolve image-subpath %q: %w", subpath, err)
	}
	if _, err := os.Stat(resolved); err != nil {
		cleanup()
		if os.IsNotExist(err) {
			return specs.Mount{}, "", "", fmt.Errorf("image-subpath %q does not exist in image %q", subpath, ref)
		}
		return specs.Mount{}, "", "", fmt.Errorf("failed to stat image-subpath %q: %w", subpath, err)
	}
	return specs.Mount{
		Type:        "bind",
		Source:      resolved,
		Destination: destination,
		Options:     []string{"rbind", "ro"},
	}, snapshotKey, hostMountpoint, nil
}

// removeImageMounts tears down type=image mount state for a container: it
// unmounts and removes any host materialization directories (image-subpath),
// then removes the read-only snapshot views. NotFound is ignored; other
// failures are logged but not fatal.
func removeImageMounts(ctx context.Context, s snapshots.Snapshotter, hostpaths, snapshotKeys []string) {
	// Unmount host materializations before removing the views they hold open.
	for _, p := range hostpaths {
		if err := mount.UnmountAll(p, 0); err != nil {
			log.G(ctx).WithError(err).Warnf("failed to unmount image-mount host path %q", p)
		}
		if err := os.RemoveAll(p); err != nil {
			log.G(ctx).WithError(err).Warnf("failed to remove image-mount host path %q", p)
		}
	}
	for _, k := range snapshotKeys {
		if err := s.Remove(ctx, k); err != nil && !errdefs.IsNotFound(err) {
			log.G(ctx).WithError(err).Warnf("failed to remove image-mount snapshot %q", k)
		}
	}
}

// generateMountOpts generates volume-related mount opts.
// Other mounts such as procfs mount are not handled here.
func generateMountOpts(ctx context.Context, client *containerd.Client, ensuredImage *imgutil.EnsuredImage,
	volStore volumestore.VolumeStore, options types.ContainerCreateOptions) (opts []oci.SpecOpts, anonVolumes []string, mountPoints []*mountutil.Processed, retErr error) {
	//nolint:prealloc
	var (
		userMounts          []specs.Mount
		imageMountViews     []string
		imageMountHostpaths []string
	)
	// Tear down any image-mount state created here if this function fails, so a
	// partial setup does not leak snapshots or host mounts.
	defer func() {
		if retErr != nil && (len(imageMountViews) > 0 || len(imageMountHostpaths) > 0) {
			removeImageMounts(ctx, client.SnapshotService(options.GOptions.Snapshotter), imageMountHostpaths, imageMountViews)
		}
	}()
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

		mm := client.MountManager()

		active, err := mm.Activate(ctx, tempDir, mounts)
		if err == nil {
			defer mm.Deactivate(ctx, tempDir)
			mounts = active.System
		} else if !errors.Is(err, errdefs.ErrNotImplemented) {
			return nil, nil, nil, fmt.Errorf("failed to activate mounts: %w", err)
		}

		// windows has additional steps for mounting see
		// https://github.com/containerd/containerd/commit/791e175c79930a34cfbb2048fbcaa8493fd2c86b
		unmounter := func(tempDir string) {
			if uerr := mount.UnmountMounts(mounts, tempDir, 0); uerr != nil {
				log.G(ctx).Debugf("Failed to unmount snapshot %q", tempDir)
				if err == nil {
					err = uerr
				}
			}
		}

		if runtime.GOOS == "linux" {
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
			// type=image: build the read-only view now and record its snapshot
			// key for cleanup on container removal.
			if x.Type == mountutil.Image {
				m, snapshotKey, hostMountpoint, err := setupImageMount(ctx, client, options, x.Mount.Source, x.Mount.Destination, x.ImageSubpath)
				if err != nil {
					return nil, nil, nil, err
				}
				imageMountViews = append(imageMountViews, snapshotKey)
				if hostMountpoint != "" {
					imageMountHostpaths = append(imageMountHostpaths, hostMountpoint)
				}
				ociMounts[i] = m
				x.ImageMountSnapshot = snapshotKey
				x.ImageMountHostpath = hostMountpoint
				mounted[filepath.Clean(x.Mount.Destination)] = struct{}{}
				continue
			}

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

		log.G(ctx).Debugf("creating anonymous volume %q, for \"VOLUME %s\"",
			anonVolName, imgVolRaw)
		anonVol, err := volStore.CreateWithoutLock(anonVolName, []string{})
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
			// Containerd note: there is no guarantee that the containers we got from the list still exist at this point
			// If that is the case, just ignore and move on
			if errors.Is(err, errdefs.ErrNotFound) {
				log.G(ctx).Debugf("container %q is gone - ignoring", c.ID())
				continue
			}
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

			nerdctlMounts := labels.GetMount(ls)
			if nerdctlMounts != "" {
				err = json.Unmarshal([]byte(nerdctlMounts), &vfMountPoints)
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
		log.L.Debugf("volume at %q is not initially empty, skipping copying", destination)
		return nil
	}
	return fs.CopyDir(destination, source)
}
