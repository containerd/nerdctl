# EROFS Image Conversion

EROFS is a read-only filesystem supported by containerd's `erofs` snapshotter and differ. nerdctl can convert image layers to EROFS media types with `nerdctl image convert --erofs`.

## Prerequisites

- Install containerd with the `erofs` snapshotter and differ plugins enabled.
- Install `mkfs.erofs` for `nerdctl image convert --erofs`.

Check that containerd has loaded the EROFS plugins:

```console
ctr plugins ls | grep erofs
```

## Configure containerd transfer unpack

containerd 2.3+ provides an EROFS unpack configuration by default when the `erofs` snapshotter and differ plugins are available.

If `plugins."io.containerd.transfer.v1.local".unpack_config` is configured manually, add an EROFS entry to `/etc/containerd/config.toml` and restart containerd:

```toml
[[plugins."io.containerd.transfer.v1.local".unpack_config]]
  platform = "linux(+erofs)/amd64"
  snapshotter = "erofs"
  differ = "erofs"
```

Replace `amd64` with the target architecture as needed. The `linux(+erofs)/ARCH` entry also allows the `erofs` snapshotter to unpack regular `linux/ARCH` tar/gzip images.

## Convert an image

Convert an image to raw EROFS blobs:

```console
nerdctl image convert --erofs raw example.com/foo:latest example.com/foo:erofs
```

Convert an image to zstd-compressed EROFS blobs:

```console
nerdctl image convert --erofs zstd example.com/foo:latest example.com/foo:erofs-zstd
```

`--erofs-compressors` passes compressor options to `mkfs.erofs`, and `--erofs-mkfs-options` passes extra `mkfs.erofs` options. See [`command-reference.md`](./command-reference.md) for flag details.

## Pull and unpack with EROFS snapshotter

Push the converted image to a registry, then pull it with the `erofs` snapshotter:

```console
nerdctl image pull --snapshotter erofs example.com/foo:erofs
```
