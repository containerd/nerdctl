# Lazy-pulling using SOCI Snapshotter

SOCI Snapshotter is a containerd snapshotter plugin. It enables standard OCI images to be lazily loaded without requiring a build-time conversion step. "SOCI" is short for "Seekable OCI", and is pronounced "so-CHEE".

See https://github.com/awslabs/soci-snapshotter to learn further information.

## SOCI Index Manifest Versions

SOCI supports two index manifest versions:

- **v1**: Original format using OCI Referrers API (disabled by default in SOCI v0.10.0+)
- **v2**: New format that packages SOCI index with the image (default in SOCI v0.10.0+)

To enable v1 indices in SOCI v0.10.0+, add to `/etc/soci-snapshotter-grpc/config.toml`:
```toml
[pull_modes]
  [pull_modes.soci_v1]
    enable = true
```

For detailed information about the differences between v1 and v2, see the [SOCI Index Manifest v2 documentation](https://github.com/awslabs/soci-snapshotter/blob/main/docs/soci-index-manifest-v2.md).

## Prerequisites

- Install containerd remote snapshotter plugin (`soci-snapshotter-grpc`) from https://github.com/awslabs/soci-snapshotter/blob/main/docs/getting-started.md

- Add the following to `/etc/containerd/config.toml`:
```toml
[proxy_plugins]
  [proxy_plugins.soci]
    type = "snapshot"
    address = "/run/soci-snapshotter-grpc/soci-snapshotter-grpc.sock"
```

- Launch `containerd` and `soci-snapshotter-grpc`

## Enable SOCI for `nerdctl run` and `nerdctl pull`

| :zap: Requirement | nerdctl >= 1.5.0 |
| ----------------- | ---------------- |

- Run `nerdctl` with `--snapshotter=soci`
```console
nerdctl run -it --rm --snapshotter=soci public.ecr.aws/soci-workshop-examples/ffmpeg:latest
```

- You can also only pull the image with SOCI without running the container.
```console
nerdctl pull --snapshotter=soci public.ecr.aws/soci-workshop-examples/ffmpeg:latest
```

For images that already have SOCI indices, see https://gallery.ecr.aws/soci-workshop-examples

## Enable SOCI for `nerdctl push`

| :zap: Requirement | nerdctl >= 1.6.0 |
| ----------------- | ---------------- |

- Push the image with SOCI index. Adding `--snapshotter=soci` arg to `nerdctl pull`, `nerdctl` will create the SOCI index and push the index to same destination as the image.
```console
nerdctl push --snapshotter=soci --soci-span-size=2097152 --soci-min-layer-size=20971520 public.ecr.aws/my-registry/my-repo:latest
```
--soci-span-size and --soci-min-layer-size are two properties to customize the SOCI index. See [Command Reference](https://github.com/containerd/nerdctl/blob/377b2077bb616194a8ef1e19ccde32aa1ffd6c84/docs/command-reference.md?plain=1#L773) for further details.

> **Note**: With SOCI v0.10.0+, When using `nerdctl push --snapshotter=soci`, it creates and pushes v1 indices. When pushing a converted image (created with `nerdctl image convert --soci`), it will push v2 indices.

## Enable SOCI for `nerdctl image convert`

| :zap: Requirement | nerdctl >= 2.1.3 |
| ----------------- | ---------------- |

| :zap: Requirement | soci-snapshotter >= 0.10.0 |
| ----------------- | ---------------- |

- Convert an image to generate SOCI Index artifacts v2. Running the `nerdctl image convert` with the `--soci` flag and a `srcImg` and `dstImg`, `nerdctl` will create the SOCI v2 indices and the new image will be present in the `dstImg` address.
```console
nerdctl image convert --soci --soci-span-size=2097152 --soci-min-layer-size=20971520 public.ecr.aws/my-registry/my-repo:latest public.ecr.aws/my-registry/my-repo:soci
```
--soci-span-size and --soci-min-layer-size are two properties to customize the SOCI index. See [Command Reference](https://github.com/containerd/nerdctl/blob/377b2077bb616194a8ef1e19ccde32aa1ffd6c84/docs/command-reference.md?plain=1#L773) for further details.

The `image convert` command with `--soci` flag creates SOCI-enabled images using SOCI Index Manifest v2, which combines the SOCI index and the original image into a single artifact.
