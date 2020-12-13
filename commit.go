/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/AkihiroSuda/nerdctl/pkg/idutil"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/diff"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/rootfs"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type commitOpts struct {
	author  string
	message string
	ref     string
}

var (
	commitCommand = &cli.Command{
		Name:        "commit",
		Usage:       "[flags] CONTAINER REPOSITORY[:TAG]",
		Description: "Create a new image from a container's changes",
		Action:      commitAction,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "author",
				Aliases: []string{"a"},
				Usage:   `Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")`,
			},
			&cli.StringFlag{
				Name:    "message",
				Aliases: []string{"m"},
				Usage:   `Commit message`,
			},
		},
	}

	emptyGZLayer = digest.Digest("sha256:4f4fb700ef54461cfa02571ae0db9a0dc1e0cdb5577484a6d75e68dc38e8acc1")
)

func commitAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 2 {
		return errors.New("need container and commit image name")
	}

	opts, err := newCommitOpts(clicontext)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	return idutil.WalkContainers(ctx, client, []string{clicontext.Args().First()}, func(ctx context.Context, client *containerd.Client, _, id string) error {
		imageID, err := commitContainer(ctx, client, id, opts)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(clicontext.App.Writer, "%s\n", imageID)
		return err
	})
}

func newCommitOpts(clicontext *cli.Context) (*commitOpts, error) {
	rawRef := clicontext.Args().Get(1)

	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}

	return &commitOpts{
		author:  clicontext.String("author"),
		message: clicontext.String("message"),
		ref:     named.String(),
	}, nil
}

var emptyDigest = digest.Digest("")

func commitContainer(ctx context.Context, client *containerd.Client, id string, opts *commitOpts) (digest.Digest, error) {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return emptyDigest, err
	}

	info, err := container.Info(ctx)
	if err != nil {
		return emptyDigest, err
	}

	// NOTE: Moby uses provided rootfs to run container. It doesn't support
	// to commit container created by moby.
	baseImg, err := container.Image(ctx)
	if err != nil {
		return emptyDigest, err
	}

	baseImgConfig, err := readImageConfig(ctx, baseImg)
	if err != nil {
		return emptyDigest, err
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return emptyDigest, err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return emptyDigest, err
	}

	switch status.Status {
	case containerd.Paused, containerd.Created, containerd.Stopped:
	default:
		if err := task.Pause(ctx); err != nil {
			return emptyDigest, errors.Wrapf(err, "failed to pause container")
		}

		defer func() {
			if err := task.Resume(ctx); err != nil {
				logrus.Warnf("failed to unpause container %v: %v", id, err)
			}
		}()
	}

	var (
		cs     = client.ContentStore()
		differ = client.DiffService()
		snName = info.Snapshotter
		sn     = client.SnapshotService(snName)
	)

	// Don't gc me and clean the dirty data after 1 hour!
	ctx, done, err := client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return emptyDigest, errors.Wrapf(err, "failed to create lease for commit")
	}
	defer done(ctx)

	diffLayerDesc, diffID, err := createDiff(ctx, id, sn, cs, differ)
	if err != nil {
		return emptyDigest, errors.Wrap(err, "failed to export layer")
	}

	imageConfig, err := generateCommitImageConfig(ctx, container, diffID, opts)
	if err != nil {
		return emptyDigest, errors.Wrap(err, "failed to generate commit image config")
	}

	rootfsID := identity.ChainID(imageConfig.RootFS.DiffIDs).String()
	if err := applyDiffLayer(ctx, rootfsID, baseImgConfig, sn, differ, diffLayerDesc); err != nil {
		return emptyDigest, errors.Wrap(err, "failed to apply diff")
	}

	commitManifestDesc, configDigest, err := writeContentsForImage(ctx, cs, snName, baseImg, imageConfig, diffLayerDesc)
	if err != nil {
		return emptyDigest, err
	}

	// image create
	img := images.Image{
		Name:      opts.ref,
		Target:    commitManifestDesc,
		CreatedAt: time.Now(),
	}

	if _, err := client.ImageService().Update(ctx, img); err != nil {
		if !errdefs.IsNotFound(err) {
			return emptyDigest, err
		}

		if _, err := client.ImageService().Create(ctx, img); err != nil {
			return emptyDigest, errors.Wrapf(err, "failed to create new image %s", opts.ref)
		}
	}
	return configDigest, nil
}

// generateCommitImageConfig returns commit oci image config based on the container's image.
func generateCommitImageConfig(ctx context.Context, container containerd.Container, diffID digest.Digest, opts *commitOpts) (ocispec.Image, error) {
	spec, err := container.Spec(ctx)
	if err != nil {
		return ocispec.Image{}, err
	}

	img, err := container.Image(ctx)
	if err != nil {
		return ocispec.Image{}, err
	}

	baseConfig, err := readImageConfig(ctx, img)
	if err != nil {
		return ocispec.Image{}, err
	}

	if opts.author == "" {
		opts.author = baseConfig.Author
	}

	createdBy := ""
	if spec.Process != nil {
		createdBy = strings.Join(spec.Process.Args, " ")
	}

	createdTime := time.Now()
	return ocispec.Image{
		Architecture: runtime.GOARCH,
		OS:           runtime.GOOS,
		Created:      &createdTime,
		Author:       opts.author,
		Config:       baseConfig.Config, // TODO(fuweid): how to update the USER/ENV/CMD/... fields?
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: append(baseConfig.RootFS.DiffIDs, diffID),
		},
		History: append(baseConfig.History, ocispec.History{
			Created:    &createdTime,
			CreatedBy:  createdBy,
			Author:     opts.author,
			Comment:    opts.message,
			EmptyLayer: (diffID == emptyGZLayer),
		}),
	}, nil
}

// writeContentsForImage will commit oci image config and manifest into containerd's content store.
func writeContentsForImage(ctx context.Context, cs content.Store, snName string, baseImg containerd.Image, newConfig ocispec.Image, diffLayerDesc ocispec.Descriptor) (ocispec.Descriptor, digest.Digest, error) {
	newConfigJSON, err := json.Marshal(newConfig)
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	configDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Config,
		Digest:    digest.FromBytes(newConfigJSON),
		Size:      int64(len(newConfigJSON)),
	}

	baseMfst, err := images.Manifest(ctx, cs, baseImg.Target(), platforms.Default())
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}
	layers := append(baseMfst.Layers, diffLayerDesc)

	newMfst := struct {
		MediaType string `json:"mediaType,omitempty"`
		ocispec.Manifest
	}{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Manifest: ocispec.Manifest{
			Versioned: specs.Versioned{
				SchemaVersion: 2,
			},
			Config: configDesc,
			Layers: layers,
		},
	}

	newMfstJSON, err := json.MarshalIndent(newMfst, "", "    ")
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	newMfstDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Manifest,
		Digest:    digest.FromBytes(newMfstJSON),
		Size:      int64(len(newMfstJSON)),
	}

	// new manifest should reference the layers and config content
	labels := map[string]string{
		"containerd.io/gc.ref.content.0": configDesc.Digest.String(),
	}
	for i, l := range layers {
		labels[fmt.Sprintf("containerd.io/gc.ref.content.%d", i+1)] = l.Digest.String()
	}

	err = content.WriteBlob(ctx, cs, newMfstDesc.Digest.String(), bytes.NewReader(newMfstJSON), newMfstDesc, content.WithLabels(labels))
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	// config should reference to snapshotter
	labelOpt := content.WithLabels(map[string]string{
		fmt.Sprintf("containerd.io/gc.ref.snapshot.%s", snName): identity.ChainID(newConfig.RootFS.DiffIDs).String(),
	})
	err = content.WriteBlob(ctx, cs, configDesc.Digest.String(), bytes.NewReader(newConfigJSON), configDesc, labelOpt)
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	return newMfstDesc, configDesc.Digest, nil
}

// createDiff creates a layer diff into containerd's content store.
func createDiff(ctx context.Context, name string, sn snapshots.Snapshotter, cs content.Store, comparer diff.Comparer) (ocispec.Descriptor, digest.Digest, error) {
	newDesc, err := rootfs.CreateDiff(ctx, name, sn, comparer)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	info, err := cs.Info(ctx, newDesc.Digest)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	diffIDStr, ok := info.Labels["containerd.io/uncompressed"]
	if !ok {
		return ocispec.Descriptor{}, digest.Digest(""), errors.Errorf("invalid differ response with no diffID")
	}

	diffID, err := digest.Parse(diffIDStr)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	return ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2LayerGzip,
		Digest:    newDesc.Digest,
		Size:      info.Size,
	}, diffID, nil
}

// applyDiffLayer will apply diff layer content created by createDiff into the snapshotter.
func applyDiffLayer(ctx context.Context, name string, baseImg ocispec.Image, sn snapshots.Snapshotter, differ diff.Applier, diffDesc ocispec.Descriptor) (retErr error) {
	var (
		key    = uniquePart() + "-" + name
		parent = identity.ChainID(baseImg.RootFS.DiffIDs).String()
	)

	mount, err := sn.Prepare(ctx, key, parent)
	if err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			// NOTE: the snapshotter should be hold by lease. Even
			// if the cleanup fails, the containerd gc can delete it.
			if err := sn.Remove(ctx, key); err != nil {
				logrus.Warnf("failed to cleanup aborted apply %s: %s", key, err)
			}
		}
	}()

	if _, err = differ.Apply(ctx, diffDesc, mount); err != nil {
		return err
	}

	if err = sn.Commit(ctx, name, key); err != nil {
		if errdefs.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}

// readImageConfig reads config spec from content store.
func readImageConfig(ctx context.Context, img containerd.Image) (ocispec.Image, error) {
	var config ocispec.Image

	configDesc, err := img.Config(ctx)
	if err != nil {
		return config, err
	}

	p, err := content.ReadBlob(ctx, img.ContentStore(), configDesc)
	if err != nil {
		return config, err
	}

	if err := json.Unmarshal(p, &config); err != nil {
		return config, err
	}
	return config, nil
}

// copied from github.com/containerd/containerd/rootfs/apply.go
func uniquePart() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.Nanosecond(), base64.URLEncoding.EncodeToString(b[:]))
}
