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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opencontainers/image-spec/identity"
	"github.com/spf13/cobra"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/continuity/fs"
	"github.com/containerd/log"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/idgen"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

func NewDiffCommand() *cobra.Command {
	var diffCommand = &cobra.Command{
		Use:               "diff [CONTAINER]",
		Short:             "Inspect changes to files or directories on a container's filesystem",
		Args:              cobra.MinimumNArgs(1),
		RunE:              diffAction,
		ValidArgsFunction: diffShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return diffCommand
}

func processContainerDiffOptions(cmd *cobra.Command) (types.ContainerDiffOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerDiffOptions{}, err
	}

	return types.ContainerDiffOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
	}, nil
}

func diffAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerDiffOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			changes, err := getChanges(ctx, client, found.Container)
			if err != nil {
				return err
			}

			for _, change := range changes {
				switch change.Kind {
				case fs.ChangeKindAdd:
					fmt.Fprintln(options.Stdout, "A", change.Path)
				case fs.ChangeKindModify:
					fmt.Fprintln(options.Stdout, "C", change.Path)
				case fs.ChangeKindDelete:
					fmt.Fprintln(options.Stdout, "D", change.Path)
				default:
				}
			}

			return nil
		},
	}

	container := args[0]

	n, err := walker.Walk(ctx, container)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", container)
	}
	return nil
}

func getChanges(ctx context.Context, client *containerd.Client, container containerd.Container) ([]fs.Change, error) {
	id := container.ID()
	info, err := container.Info(ctx)
	if err != nil {
		return nil, err
	}

	var (
		snName = info.Snapshotter
		sn     = client.SnapshotService(snName)
	)

	mounts, err := sn.Mounts(ctx, id)
	if err != nil {
		return nil, err
	}

	// NOTE: Moby uses provided rootfs to run container. It doesn't support
	// to commit container created by moby.
	baseImgWithoutPlatform, err := client.ImageService().Get(ctx, info.Image)
	if err != nil {
		return nil, fmt.Errorf("container %q lacks image (wasn't created by nerdctl?): %w", id, err)
	}
	platformLabel := info.Labels[labels.Platform]
	if platformLabel == "" {
		platformLabel = platforms.DefaultString()
		log.G(ctx).Warnf("Image lacks label %q, assuming the platform to be %q", labels.Platform, platformLabel)
	}
	ocispecPlatform, err := platforms.Parse(platformLabel)
	if err != nil {
		return nil, err
	}
	log.G(ctx).Debugf("ocispecPlatform=%q", platforms.Format(ocispecPlatform))
	platformMC := platforms.Only(ocispecPlatform)
	baseImg := containerd.NewImageWithPlatform(client, baseImgWithoutPlatform, platformMC)

	baseImgConfig, _, err := imgutil.ReadImageConfig(ctx, baseImg)
	if err != nil {
		return nil, err
	}

	// Don't gc me and clean the dirty data after 1 hour!
	ctx, done, err := client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease for diff: %w", err)
	}
	defer done(ctx)

	rootfsID := identity.ChainID(baseImgConfig.RootFS.DiffIDs).String()

	randomID := idgen.GenerateID()
	parent, err := sn.View(ctx, randomID, rootfsID)
	if err != nil {
		return nil, err
	}
	defer sn.Remove(ctx, randomID)

	var changes []fs.Change
	err = mount.WithReadonlyTempMount(ctx, parent, func(lower string) error {
		return mount.WithReadonlyTempMount(ctx, mounts, func(upper string) error {
			return fs.Changes(ctx, lower, upper, func(ck fs.ChangeKind, s string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				changes = appendChanges(changes, fs.Change{
					Kind: ck,
					Path: s,
				})
				return nil
			})
		})
	})
	if err != nil {
		return nil, err
	}

	return changes, err
}

func appendChanges(changes []fs.Change, new fs.Change) []fs.Change {
	newDir, _ := filepath.Split(new.Path)
	newDirPath := filepath.SplitList(newDir)

	if len(changes) == 0 {
		for i := 1; i < len(newDirPath); i++ {
			changes = append(changes, fs.Change{
				Kind: fs.ChangeKindModify,
				Path: filepath.Join(newDirPath[:i+1]...),
			})
		}
		return append(changes, new)
	}
	last := changes[len(changes)-1]
	lastDir, _ := filepath.Split(last.Path)
	lastDirPath := filepath.SplitList(lastDir)
	for i := range newDirPath {
		if len(lastDirPath) > i && lastDirPath[i] == newDirPath[i] {
			continue
		}
		changes = append(changes, fs.Change{
			Kind: fs.ChangeKindModify,
			Path: filepath.Join(newDirPath[:i+1]...),
		})
	}
	return append(changes, new)
}

func diffShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return completion.ContainerNames(cmd, nil)
}
