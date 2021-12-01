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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/continuity/fs"
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

// parseMountFlags parses --volume and --tmpfs.
// parseMountFlags will also parse --mount in a future release.
func parseMountFlags(cmd *cobra.Command, volStore volumestore.VolumeStore) ([]*mountutil.Processed, error) {
	var parsed []*mountutil.Processed
	if flagVSlice, err := cmd.Flags().GetStringArray("volume"); err != nil {
		return nil, err
	} else {
		for _, v := range strutil.DedupeStrSlice(flagVSlice) {
			x, err := mountutil.ProcessFlagV(v, volStore)
			if err != nil {
				return nil, err
			}
			parsed = append(parsed, x)
		}
	}

	// tmpfs needs to be StringArray, not StringSlice, to prevent "/foo:size=64m,exec" from being split to {"/foo:size=64m", "exec"}
	if tmpfsSlice, err := cmd.Flags().GetStringArray("tmpfs"); err != nil {
		return nil, err
	} else {
		for _, v := range strutil.DedupeStrSlice(tmpfsSlice) {
			x, err := mountutil.ProcessFlagTmpfs(v)
			if err != nil {
				return nil, err
			}
			parsed = append(parsed, x)
		}
	}
	return parsed, nil
}

// generateMountOpts generates volume-related mount opts.
// Other mounts such as procfs mount are not handled here.
func generateMountOpts(cmd *cobra.Command, ctx context.Context, client *containerd.Client, ensuredImage *imgutil.EnsuredImage) ([]oci.SpecOpts, []string, error) {
	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return nil, nil, err
	}

	//nolint:golint,prealloc
	var (
		opts        []oci.SpecOpts
		anonVolumes []string
	)
	mounted := make(map[string]struct{})
	var imageVolumes map[string]struct{}
	var tempDir string

	if ensuredImage != nil {
		imageVolumes = ensuredImage.ImageConfig.Volumes

		snapshotter, err := cmd.Flags().GetString("snapshotter")
		if err != nil {
			return nil, nil, err
		}

		if err := ensuredImage.Image.Unpack(ctx, snapshotter); err != nil {
			return nil, nil, fmt.Errorf("error unpacking image: %w", err)
		}

		diffIDs, err := ensuredImage.Image.RootFS(ctx)
		if err != nil {
			return nil, nil, err
		}
		chainID := identity.ChainID(diffIDs).String()

		s := client.SnapshotService(snapshotter)
		tempDir, err = os.MkdirTemp("", "initialC")
		if err != nil {
			return nil, nil, err
		}
		// We use Remove here instead of RemoveAll.
		// The RemoveAll will delete the temp dir and all children it contains.
		// When the Unmount fails, RemoveAll will incorrectly delete data from the mounted dir
		defer os.Remove(tempDir)

		var mounts []mount.Mount
		mounts, err = s.View(ctx, tempDir, chainID)
		if err != nil {
			return nil, nil, err
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
						return nil, nil, err
					}
					return nil, nil, err
				}
			}
		} else {
			defer unmounter(tempDir)
			if err := mount.All(mounts, tempDir); err != nil {
				if err := s.Remove(ctx, tempDir); err != nil && !errdefs.IsNotFound(err) {
					return nil, nil, err
				}
				return nil, nil, err
			}
		}
	}

	if parsed, err := parseMountFlags(cmd, volStore); err != nil {
		return nil, nil, err
	} else if len(parsed) > 0 {
		ociMounts := make([]specs.Mount, len(parsed))
		for i, x := range parsed {
			ociMounts[i] = x.Mount
			mounted[filepath.Clean(x.Mount.Destination)] = struct{}{}

			target, err := securejoin.SecureJoin(tempDir, x.Mount.Destination)
			if err != nil {
				return nil, nil, err
			}

			//Coyping content in AnonymousVolume and namedVolume
			if x.Type == "volume" {
				if err := copyExistingContents(target, x.Mount.Source); err != nil {
					return nil, nil, err
				}
			}
			if x.AnonymousVolume != "" {
				anonVolumes = append(anonVolumes, x.AnonymousVolume)
			}
			opts = append(opts, x.Opts...)
		}
		opts = append(opts, oci.WithMounts(ociMounts))
	}

	// imageVolumes are defined in Dockerfile "VOLUME" instruction
	for imgVolRaw := range imageVolumes {
		imgVol := filepath.Clean(imgVolRaw)
		switch imgVol {
		case "/", "/dev", "/sys", "proc":
			return nil, nil, fmt.Errorf("invalid VOLUME: %q", imgVolRaw)
		}
		if _, ok := mounted[imgVol]; ok {
			continue
		}
		anonVolName := idgen.GenerateID()

		logrus.Debugf("creating anonymous volume %q, for \"VOLUME %s\"",
			anonVolName, imgVolRaw)
		anonVol, err := volStore.Create(anonVolName, []string{})
		if err != nil {
			return nil, nil, err
		}

		target, err := securejoin.SecureJoin(tempDir, imgVol)
		if err != nil {
			return nil, nil, err
		}

		//copying up initial contents of the mount point directory
		if err := copyExistingContents(target, anonVol.Mountpoint); err != nil {
			return nil, nil, err
		}

		m := []specs.Mount{
			{
				Type:        "none",
				Source:      anonVol.Mountpoint,
				Destination: imgVol,
				Options:     []string{"rbind"},
			},
		}

		opts = append(opts, oci.WithMounts(m))
		anonVolumes = append(anonVolumes, anonVolName)
	}

	return opts, anonVolumes, nil
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
