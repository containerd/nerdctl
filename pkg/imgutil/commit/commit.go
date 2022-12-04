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

package commit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/diff"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots"
	imgutil "github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

type Changes struct {
	CMD, Entrypoint []string
}

type Opts struct {
	Author            string
	Message           string
	Ref               string
	Pause             bool
	Changes           Changes
	Image             string
	LowerDir          string
	UpperDir          string
	ExcludeDir        string
	ExcludeRootfsDirs []string
}

var (
	emptyGZLayer = digest.Digest("sha256:4f4fb700ef54461cfa02571ae0db9a0dc1e0cdb5577484a6d75e68dc38e8acc1")
	emptyDigest  = digest.Digest("")

	emptyDesc    = ocispec.Descriptor{}
	uncompressed = "containerd.io/uncompressed"
)

func Commit(ctx context.Context, client *containerd.Client, opts *Opts) (digest.Digest, error) {
	// NOTE: Moby uses provided rootfs to run container. It doesn't support
	// to commit container created by moby.
	baseImgWithoutPlatform, err := client.ImageService().Get(ctx, opts.Image)
	if err != nil {
		return emptyDigest, fmt.Errorf("container lacks image (wasn't created by nerdctl?): %w", err)
	}
	platformLabel := platforms.DefaultString()
	logrus.Warnf("Image lacks label %q, assuming the platform to be %q", labels.Platform, platformLabel)
	ocispecPlatform, err := platforms.Parse(platformLabel)
	if err != nil {
		return emptyDigest, err
	}
	logrus.Debugf("ocispecPlatform=%q", platforms.Format(ocispecPlatform))
	platformMC := platforms.Only(ocispecPlatform)
	baseImg := containerd.NewImageWithPlatform(client, baseImgWithoutPlatform, platformMC)

	baseImgConfig, _, err := imgutil.ReadImageConfig(ctx, baseImg)
	if err != nil {
		return emptyDigest, err
	}

	var (
		differ = client.DiffService()
		snName = "overlayfs"
		sn     = client.SnapshotService(snName)
	)

	// Don't gc me and clean the dirty data after 1 hour!
	ctx, done, err := client.WithLease(ctx, leases.WithRandomID(), leases.WithExpiration(1*time.Hour))
	if err != nil {
		return emptyDigest, fmt.Errorf("failed to create lease for commit: %w", err)
	}
	defer done(ctx)

	lower := []mount.Mount{
		{
			Type:   "bind",
			Source: opts.LowerDir,
			Options: []string{
				"ro", "rbind",
			},
		},
	}

	rootfs := []mount.Mount{
		{
			Type:   "overlay",
			Source: "overlay",
			Options: []string{
				"index=off",
				"nfs_export=off",
				fmt.Sprintf("lowerdir=%s", opts.LowerDir),
				fmt.Sprintf("upperdir=%s", opts.UpperDir),
				"workdir=/ebs/work",
			},
		},
	}
	exclude := []mount.Mount{}
	if len(opts.ExcludeRootfsDirs) > 0 {
		for _, excludeRootfsDir := range opts.ExcludeRootfsDirs {
			exclude = append(exclude, mount.Mount{
				Type:   "bind",
				Source: path.Join(opts.ExcludeDir, excludeRootfsDir),
				Options: []string{
					"ro", "rbind",
				},
			})
		}
	}

	diffLayerDesc, diffID, err := createDiff(ctx, client.ContentStore(), differ, lower, rootfs, exclude, opts.ExcludeDir)
	if err != nil {
		return emptyDigest, fmt.Errorf("failed to export layer: %w", err)
	}

	imageConfig, err := generateCommitImageConfig(ctx, baseImg, diffID, opts)
	if err != nil {
		return emptyDigest, fmt.Errorf("failed to generate commit image config: %w", err)
	}

	rootfsID := identity.ChainID(imageConfig.RootFS.DiffIDs).String()
	if err := applyDiffLayer(ctx, rootfsID, baseImgConfig, sn, differ, diffLayerDesc); err != nil {
		return emptyDigest, fmt.Errorf("failed to apply diff: %w", err)
	}

	commitManifestDesc, configDigest, err := writeContentsForImage(ctx, snName, baseImg, imageConfig, diffLayerDesc)
	if err != nil {
		return emptyDigest, err
	}

	// image create
	img := images.Image{
		Name:      opts.Ref,
		Target:    commitManifestDesc,
		CreatedAt: time.Now(),
	}

	if _, err := client.ImageService().Update(ctx, img); err != nil {
		if !errdefs.IsNotFound(err) {
			return emptyDigest, err
		}

		if _, err := client.ImageService().Create(ctx, img); err != nil {
			return emptyDigest, fmt.Errorf("failed to create new image %s: %w", opts.Ref, err)
		}
	}
	return configDigest, nil
}

// generateCommitImageConfig returns commit oci image config based on the container's image.
func generateCommitImageConfig(ctx context.Context, img containerd.Image, diffID digest.Digest, opts *Opts) (ocispec.Image, error) {

	baseConfig, _, err := imgutil.ReadImageConfig(ctx, img) // aware of img.platform
	if err != nil {
		return ocispec.Image{}, err
	}

	// TODO(fuweid): support updating the USER/ENV/... fields?
	if opts.Changes.CMD != nil {
		baseConfig.Config.Cmd = opts.Changes.CMD
	}
	if opts.Changes.Entrypoint != nil {
		baseConfig.Config.Entrypoint = opts.Changes.Entrypoint
	}
	if opts.Author == "" {
		opts.Author = baseConfig.Author
	}

	createdTime := time.Now()
	arch := baseConfig.Architecture
	if arch == "" {
		arch = runtime.GOARCH
		logrus.Warnf("assuming arch=%q", arch)
	}
	os := baseConfig.OS
	if os == "" {
		os = runtime.GOOS
		logrus.Warnf("assuming os=%q", os)
	}
	logrus.Debugf("generateCommitImageConfig(): arch=%q, os=%q", arch, os)
	return ocispec.Image{
		Architecture: arch,
		OS:           os,

		Created: &createdTime,
		Author:  opts.Author,
		Config:  baseConfig.Config,
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: append(baseConfig.RootFS.DiffIDs, diffID),
		},
		History: append(baseConfig.History, ocispec.History{
			Created:    &createdTime,
			CreatedBy:  strings.Join([]string{"sh", "-c", "sh /mlplatform/setup.sh"}, " "),
			Author:     opts.Author,
			Comment:    opts.Message,
			EmptyLayer: (diffID == emptyGZLayer),
		}),
	}, nil
}

// writeContentsForImage will commit oci image config and manifest into containerd's content store.
func writeContentsForImage(ctx context.Context, snName string, baseImg containerd.Image, newConfig ocispec.Image, diffLayerDesc ocispec.Descriptor) (ocispec.Descriptor, digest.Digest, error) {
	newConfigJSON, err := json.Marshal(newConfig)
	if err != nil {
		return ocispec.Descriptor{}, emptyDigest, err
	}

	configDesc := ocispec.Descriptor{
		MediaType: images.MediaTypeDockerSchema2Config,
		Digest:    digest.FromBytes(newConfigJSON),
		Size:      int64(len(newConfigJSON)),
	}

	cs := baseImg.ContentStore()
	baseMfst, _, err := imgutil.ReadManifest(ctx, baseImg)
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
func createDiff(ctx context.Context, cs content.Store, comparer diff.Comparer, lower, rootfs, exclude []mount.Mount, excludeDir string) (ocispec.Descriptor, digest.Digest, error) {
	newDesc, err := Compare(ctx, cs, lower, rootfs, exclude, excludeDir)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	info, err := cs.Info(ctx, newDesc.Digest)
	if err != nil {
		return ocispec.Descriptor{}, digest.Digest(""), err
	}

	diffIDStr, ok := info.Labels["containerd.io/uncompressed"]
	if !ok {
		return ocispec.Descriptor{}, digest.Digest(""), fmt.Errorf("invalid differ response with no diffID")
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

// copied from github.com/containerd/containerd/rootfs/apply.go
func uniquePart() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.Nanosecond(), base64.URLEncoding.EncodeToString(b[:]))
}

// Compare creates a diff between the given mounts and uploads the result
// to the content store.
func Compare(ctx context.Context, cs content.Store, lower, upper, exclude []mount.Mount, excludeDir string, opts ...diff.Opt) (d ocispec.Descriptor, err error) {
	var config diff.Config
	for _, opt := range opts {
		if err := opt(&config); err != nil {
			return emptyDesc, err
		}
	}

	var isCompressed bool
	if config.Compressor != nil {
		if config.MediaType == "" {
			return emptyDesc, errors.New("media type must be explicitly specified when using custom compressor")
		}
		isCompressed = true
	} else {
		if config.MediaType == "" {
			config.MediaType = ocispec.MediaTypeImageLayerGzip
		}

		switch config.MediaType {
		case ocispec.MediaTypeImageLayer:
		case ocispec.MediaTypeImageLayerGzip:
			isCompressed = true
		default:
			return emptyDesc, fmt.Errorf("unsupported diff media type: %v: %w", config.MediaType, errdefs.ErrNotImplemented)
		}
	}

	var ocidesc ocispec.Descriptor
	if err := WithTempMount(ctx, lower, nil, "", func(lowerRoot string) error {
		return WithTempMount(ctx, upper, exclude, excludeDir, func(upperRoot string) error {
			var newReference bool
			if config.Reference == "" {
				newReference = true
				config.Reference = uniqueRef()
			}

			cw, err := cs.Writer(ctx,
				content.WithRef(config.Reference),
				content.WithDescriptor(ocispec.Descriptor{
					MediaType: config.MediaType, // most contentstore implementations just ignore this
				}))
			if err != nil {
				return fmt.Errorf("failed to open writer: %w", err)
			}

			// errOpen is set when an error occurs while the content writer has not been
			// committed or closed yet to force a cleanup
			var errOpen error
			defer func() {
				if errOpen != nil {
					cw.Close()
					if newReference {
						if abortErr := cs.Abort(ctx, config.Reference); abortErr != nil {
							log.G(ctx).WithError(abortErr).WithField("ref", config.Reference).Warnf("failed to delete diff upload")
						}
					}
				}
			}()
			if !newReference {
				if errOpen = cw.Truncate(0); errOpen != nil {
					return errOpen
				}
			}

			if isCompressed {
				dgstr := digest.SHA256.Digester()
				var compressed io.WriteCloser
				if config.Compressor != nil {
					compressed, errOpen = config.Compressor(cw, config.MediaType)
					if errOpen != nil {
						return fmt.Errorf("failed to get compressed stream: %w", errOpen)
					}
				} else {
					compressed, errOpen = compression.CompressStream(cw, compression.Gzip)
					if errOpen != nil {
						return fmt.Errorf("failed to get compressed stream: %w", errOpen)
					}
				}
				errOpen = archive.WriteDiff(ctx, io.MultiWriter(compressed, dgstr.Hash()), lowerRoot, upperRoot)
				compressed.Close()
				if errOpen != nil {
					return fmt.Errorf("failed to write compressed diff: %w", errOpen)
				}

				if config.Labels == nil {
					config.Labels = map[string]string{}
				}
				config.Labels[uncompressed] = dgstr.Digest().String()
			} else {
				if errOpen = archive.WriteDiff(ctx, cw, lowerRoot, upperRoot); errOpen != nil {
					return fmt.Errorf("failed to write diff: %w", errOpen)
				}
			}

			var commitopts []content.Opt
			if config.Labels != nil {
				commitopts = append(commitopts, content.WithLabels(config.Labels))
			}

			dgst := cw.Digest()
			if errOpen = cw.Commit(ctx, 0, dgst, commitopts...); errOpen != nil {
				if !errdefs.IsAlreadyExists(errOpen) {
					return fmt.Errorf("failed to commit: %w", errOpen)
				}
				errOpen = nil
			}

			info, err := cs.Info(ctx, dgst)
			if err != nil {
				return fmt.Errorf("failed to get info from content store: %w", err)
			}
			if info.Labels == nil {
				info.Labels = make(map[string]string)
			}
			// Set uncompressed label if digest already existed without label
			if _, ok := info.Labels[uncompressed]; !ok {
				info.Labels[uncompressed] = config.Labels[uncompressed]
				if _, err := cs.Update(ctx, info, "labels."+uncompressed); err != nil {
					return fmt.Errorf("error setting uncompressed label: %w", err)
				}
			}

			ocidesc = ocispec.Descriptor{
				MediaType: config.MediaType,
				Size:      info.Size,
				Digest:    info.Digest,
			}
			return nil
		})
	}); err != nil {
		return emptyDesc, err
	}

	return ocidesc, nil
}

func uniqueRef() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.UnixNano(), base64.URLEncoding.EncodeToString(b[:]))
}

var tempMountLocation = getTempDir()

// WithTempMount mounts the provided mounts to a temp dir, and pass the temp dir to f.
// The mounts are valid during the call to the f.
// Finally we will unmount and remove the temp dir regardless of the result of f.
func WithTempMount(ctx context.Context, mounts, exclude []mount.Mount, excludeDir string, f func(root string) error) (err error) {
	root, uerr := os.MkdirTemp(tempMountLocation, "containerd-mount")
	if uerr != nil {
		return fmt.Errorf("failed to create temp dir: %w", uerr)
	}
	// We use Remove here instead of RemoveAll.
	// The RemoveAll will delete the temp dir and all children it contains.
	// When the Unmount fails, RemoveAll will incorrectly delete data from
	// the mounted dir. However, if we use Remove, even though we won't
	// successfully delete the temp dir and it may leak, we won't loss data
	// from the mounted dir.
	// For details, please refer to #1868 #1785.
	defer func() {
		if uerr = os.Remove(root); uerr != nil {
			log.G(ctx).WithError(uerr).WithField("dir", root).Error("failed to remove mount temp dir")
		}
	}()

	// We should do defer first, if not we will not do Unmount when only a part of Mounts are failed.
	defer func() {
		if uerr = mount.UnmountAll(root, 0); uerr != nil {
			uerr = fmt.Errorf("failed to unmount %s: %w", root, uerr)
			if err == nil {
				err = uerr
			} else {
				err = fmt.Errorf("%s: %w", uerr.Error(), err)
			}
		}
	}()
	if uerr = mount.All(mounts, root); uerr != nil {
		return fmt.Errorf("failed to mount %s: %w", root, uerr)
	}
	if len(exclude) > 0 {
		if uerr = os.MkdirAll(excludeDir, fs.ModeDir); uerr != nil {
			return fmt.Errorf("failed to create exclude dir %s: %w", excludeDir, uerr)
		}
		defer func() {
			if uerr = os.Remove(excludeDir); uerr != nil {
				log.G(ctx).WithError(uerr).WithField("dir", root).Error(fmt.Errorf("failed to remove exclude dir %s: %w", excludeDir, uerr))
			}
		}()
		for _, em := range exclude {
			if uerr = os.MkdirAll(em.Source, fs.ModeDir); uerr != nil {
				return fmt.Errorf("failed to create exclude rootfs dir %s: %w", em.Source, uerr)
			}
			excludeRootfsDir := strings.ReplaceAll(em.Source, excludeDir, root)
			defer func() {
				if uerr = mount.UnmountAll(excludeRootfsDir, 0); uerr != nil {
					uerr = fmt.Errorf("failed to unmount %s: %w", excludeRootfsDir, uerr)
					if err == nil {
						err = uerr
					} else {
						err = fmt.Errorf("%s: %w", uerr.Error(), err)
					}
				}
			}()
			if uerr = mount.All([]mount.Mount{em}, excludeRootfsDir); uerr != nil {
				return fmt.Errorf("failed to mount %s: %w", excludeRootfsDir, uerr)
			}
		}
	}

	if err := f(root); err != nil {
		return fmt.Errorf("mount callback failed on %s: %w", root, err)
	}
	return nil
}

func getTempDir() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return xdg
	}
	return os.TempDir()
}
