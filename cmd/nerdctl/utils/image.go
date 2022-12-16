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
	"errors"
	"fmt"
	"io"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/opencontainers/image-spec/identity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func LoadImage(in io.Reader, cmd *cobra.Command, platMC platforms.MatchComparer, quiet bool) error {
	// In addition to passing WithImagePlatform() to client.Import(), we also need to pass WithDefaultPlatform() to NewClient().
	// Otherwise unpacking may fail.
	client, ctx, cancel, err := ncclient.New(cmd, containerd.WithDefaultPlatform(platMC))
	if err != nil {
		return err
	}
	defer cancel()

	sn, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return err
	}

	r := &common.ReadCounter{Reader: in}
	imgs, err := client.Import(ctx, r, containerd.WithDigestRef(archive.DigestTranslator(sn)), containerd.WithSkipDigestRef(func(name string) bool { return name != "" }), containerd.WithImportPlatform(platMC))
	if err != nil {
		if r.N == 0 {
			// Avoid confusing "unrecognized image format"
			return errors.New("no image was built")
		}
		if errors.Is(err, images.ErrEmptyWalk) {
			err = fmt.Errorf("%w (Hint: set `--platform=PLATFORM` or `--all-platforms`)", err)
		}
		return err
	}
	for _, img := range imgs {
		image := containerd.NewImageWithPlatform(client, img, platMC)

		// TODO: Show unpack status
		if !quiet {
			fmt.Fprintf(cmd.OutOrStdout(), "unpacking %s (%s)...\n", img.Name, img.Target.Digest)
		}
		err = image.Unpack(ctx, sn)
		if err != nil {
			return err
		}
		if quiet {
			fmt.Fprintln(cmd.OutOrStdout(), img.Target.Digest)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Loaded image: %s\n", img.Name)
		}
	}

	return nil
}

// UnpackedImageSize is the size of the unpacked snapshots.
// Does not contain the size of the blobs in the content store. (Corresponds to Docker).
func UnpackedImageSize(ctx context.Context, s snapshots.Snapshotter, img containerd.Image) (int64, error) {
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return 0, err
	}

	chainID := identity.ChainID(diffIDs).String()
	usage, err := s.Usage(ctx, chainID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			logrus.WithError(err).Debugf("image %q seems not unpacked", img.Name())
			return 0, nil
		}
		return 0, err
	}

	info, err := s.Stat(ctx, chainID)
	if err != nil {
		return 0, err
	}

	//Add ChainID's parent usage to the total usage
	if err := common.SnapshotKey(info.Parent).Add(ctx, s, &usage); err != nil {
		return 0, err
	}
	return usage.Size, nil
}
