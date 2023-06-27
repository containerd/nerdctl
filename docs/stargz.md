# Lazy-pulling using Stargz Snapshotter

| :zap: Requirement | nerdctl >= 0.0.1 |
|-------------------|------------------|

Lazy-pulling is a technique to running containers before completion of pulling the images.

See https://github.com/containerd/stargz-snapshotter to learn further information.

[![asciicast](https://asciinema.org/a/378377.svg)](https://asciinema.org/a/378377)

## Enable lazy-pulling for `nerdctl run`

> **NOTE**
> For rootless installation, see [`rootless.md`](./rootless.md#stargz-snapshotter)

- Install Stargz plugin (`containerd-stargz-grpc`) from https://github.com/containerd/stargz-snapshotter 

- Add the following to `/etc/containerd/config.toml`:
```toml
[proxy_plugins]
  [proxy_plugins.stargz]                                                                                  
    type = "snapshot"
    address = "/run/containerd-stargz-grpc/containerd-stargz-grpc.sock"
```

- Launch `containerd` and `containerd-stargz-grpc`

- Run `nerdctl` with `--snapshotter=stargz`
```console
# nerdctl --snapshotter=stargz run -it --rm ghcr.io/stargz-containers/fedora:30-esgz
```

For the list of pre-converted Stargz images, see https://github.com/containerd/stargz-snapshotter/blob/main/docs/pre-converted-images.md

### Benchmark result (Dec 9, 2020)
For running `python3 -c print("hi")`, eStargz with Stargz Snapshotter is 3-4 times faster than the legacy OCI with overlayfs snapshotter.

Legacy OCI with overlayfs snapshotter:
```console
# time nerdctl --snapshotter=overlayfs run -it --rm ghcr.io/stargz-containers/python:3.7-org python3 -c 'print("hi")'
ghcr.io/stargz-containers/python:3.7-org:                                         resolved       |++++++++++++++++++++++++++++++++++++++|
index-sha256:6008006c63b0a6043a11ac151cee572e0c8676b4ba3130ff23deff5f5d711237:    done           |++++++++++++++++++++++++++++++++++++++|
manifest-sha256:48eafda05f80010a6677294473d51a530e8f15375b6447195b6fb04dc2a30ce7: done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:f860607a6cd9751ac8db2f33cbc3ce1777a44eb3c04853e116763441a304fbf6:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:96b2c1e36db5f5910f58da2ca4f9311b0690810c7107fb055ee1541498b5061f:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:c495e8de12d26c9843a7a2bf8c68de1e5652e66d80d9bc869279f9af6f86736a:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:33382189822a108b249cf3ccd234d04c3a8dfe7d593df19c751dcfab3675d5f2:    done           |++++++++++++++++++++++++++++++++++++++|
config-sha256:94c9a318e47ab8a318582e2712bb495f92f17a7c1e50f13cc8a3e362c1b09290:   done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:6eaa0b6b8562fb4a02e140ae53b3910fc4d0db6e68660390eaef993f42e21102:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:adbdcbacafe93bf0791e49c8d3689bb78d9e60d02d384d4e14433aedae39f52c:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:756975cb9c7e7933d824af9319b512dd72a50894232761d06ef3be59981df838:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:d77915b4e630d47296770ce4cf481894885978072432456615172af463433cc5:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:5f37a0a41b6b03489dd7de0aa2a79e369fd8b219bbc36b52f3f9790dc128e74b:    done           |++++++++++++++++++++++++++++++++++++++|
elapsed: 41.9s                                                                    total:  321.3  (7.7 MiB/s)                                       
hi                                                                                                        
                                                     
real    0m51.754s                                                                                         
user    0m2.687s
sys     0m5.533s 
```

eStargz with Stargz Snapshotter:
```console
# time nerdctl --snapshotter=stargz run -it --rm ghcr.io/stargz-containers/python:3.7-esgz python3 -c 'print("hi")'
fetching sha256:2ea0dd96... application/vnd.oci.image.index.v1+json
fetching sha256:9612ff73... application/vnd.docker.distribution.manifest.v2+json
fetching sha256:34e5920e... application/vnd.docker.container.image.v1+json
hi

real    0m13.589s
user    0m0.132s
sys     0m0.158s
```

## Enable lazy-pulling for pulling base images during `nerdctl build`

- Launch `buildkitd` with `--oci-worker-snapshotter=stargz` (or `--containerd-worker-snapshotter=stargz` if you use containerd worker)
- Launch `nerdctl build`. No need to specify `--snapshotter` for `nerdctl`.

## Building stargz images using `nerdctl build`

```console
$ nerdctl build -t example.com/foo .
$ nerdctl image convert --estargz --oci example.com/foo example.com/foo:estargz
$ nerdctl push example.com/foo:estargz
```

NOTE: `--estargz` should be specified in conjunction with `--oci`

Stargz Snapshotter is not needed for building stargz images.

## Tips for image conversion

### Tips 1: Creating smaller eStargz images

`nerdctl image convert` allows the following flags for optionally creating a smaller eStargz image.
The result image requires stargz-snapshotter >= v0.13.0 for lazy pulling.

- `--estargz-min-chunk-size`: The minimal number of bytes of data must be written in one gzip stream. If it's > 0, multiple files and chunks can be written into one gzip stream. Smaller number of gzip header and smaller size of the result blob can be expected. `--estargz-min-chunk-size=0` produces normal eStargz.

- `--estargz-external-toc`: Separate TOC JSON metadata into another image (called "TOC image"). The result eStargz doesn't contain TOC so we can expect a smaller size than normal eStargz. This is an [experimental](./experimental.md) feature.

#### `--estargz-min-chunk-size` usage

conversion:

```console
# nerdctl image convert --oci --estargz --estargz-min-chunk-size=50000 ghcr.io/stargz-containers/ubuntu:22.04 registry2:5000/ubuntu:22.04-chunk50000
# nerdctl image ls
REPOSITORY                          TAG                 IMAGE ID        CREATED           PLATFORM       SIZE        BLOB SIZE
ghcr.io/stargz-containers/ubuntu    22.04               20fa2d7bb4de    14 seconds ago    linux/amd64    83.4 MiB    29.0 MiB
registry2:5000/ubuntu               22.04-chunk50000    562e09e1b3c1    2 seconds ago     linux/amd64    0.0 B       29.2 MiB
# nerdctl push --insecure-registry registry2:5000/ubuntu:22.04-chunk50000
```

Pull it lazily:

```console
# nerdctl pull --snapshotter=stargz --insecure-registry registry2:5000/ubuntu:22.04-chunk50000
# mount | grep "stargz on"
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/1/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
```

#### `--estargz-external-toc` usage

convert:

```console
# nerdctl image convert --oci --estargz --estargz-external-toc ghcr.io/stargz-containers/ubuntu:22.04 registry2:5000/ubuntu:22.04-ex
INFO[0005] Extra image(0) registry2:5000/ubuntu:22.04-ex-esgztoc
sha256:3059dd5d9c404344e0b7c43d9782de8cae908531897262b7772103a0b585bbee
# nerdctl images
REPOSITORY                          TAG                 IMAGE ID        CREATED           PLATFORM       SIZE        BLOB SIZE
ghcr.io/stargz-containers/ubuntu    22.04                20fa2d7bb4de    9 seconds ago    linux/amd64    83.4 MiB    29.0 MiB
registry2:5000/ubuntu               22.04-ex             3059dd5d9c40    1 second ago     linux/amd64    0.0 B       30.8 MiB
registry2:5000/ubuntu               22.04-ex-esgztoc     18c042b6eb8b    1 second ago     linux          0.0 B       151.3 KiB
```

Then push eStargz(`registry2:5000/ubuntu:22.04-ex`) and TOC image(`registry2:5000/ubuntu:22.04-ex-esgztoc`) to the same registry (`registry2` is used in this example but you can use arbitrary registries):

```console
# nerdctl push --insecure-registry registry2:5000/ubuntu:22.04-ex
# nerdctl push --insecure-registry registry2:5000/ubuntu:22.04-ex-esgztoc
```

Pull it lazily:

```console
# nerdctl pull --insecure-registry --snapshotter=stargz registry2:5000/ubuntu:22.04-ex
```

Stargz Snapshotter automatically refers to the TOC image on the same registry.

##### optional `--estargz-keep-diff-id` flag for conversion without changing layer diffID

`nerdctl image convert` supports optional flag `--estargz-keep-diff-id` specified with `--estargz-external-toc`.
This converts an image to eStargz without changing the diffID (uncompressed digest) so even eStargz-agnostic gzip decompressor (e.g. gunzip) can restore the original tar blob.

```console
# nerdctl image convert --oci --estargz --estargz-external-toc --estargz-keep-diff-id ghcr.io/stargz-containers/ubuntu:22.04 registry2:5000/ubuntu:22.04-ex-keepdiff
# nerdctl push --insecure-registry registry2:5000/ubuntu:22.04-ex-keepdiff
# nerdctl push --insecure-registry registry2:5000/ubuntu:22.04-ex-keepdiff-esgztoc
# crane --insecure blob registry2:5000/ubuntu:22.04-ex-keepdiff@sha256:2dc39ba059dcd42ade30aae30147b5692777ba9ff0779a62ad93a74de02e3e1f | jq -r '.rootfs.diff_ids[]'
sha256:7f5cbd8cc787c8d628630756bcc7240e6c96b876c2882e6fc980a8b60cdfa274
# crane blob ghcr.io/stargz-containers/ubuntu:22.04@sha256:2dc39ba059dcd42ade30aae30147b5692777ba9ff0779a62ad93a74de02e3e1f | jq -r '.rootfs.diff_ids[]'
sha256:7f5cbd8cc787c8d628630756bcc7240e6c96b876c2882e6fc980a8b60cdfa274
```

### Tips 2: Using zstd instead of gzip (a.k.a. zstd:chunked)

You can use zstd compression with lazy pulling support (a.k.a zstd:chunked) instead of gzip.

- Pros
  - [Faster](https://github.com/facebook/zstd/tree/v1.5.2#benchmarks) compression/decompression.
- Cons
  - Old tools might not support. And unsupported by some tools yet.
    - zstd supported by OCI Image Specification is still under rc (2022/11). will be added to [v1.1.0](https://github.com/opencontainers/image-spec/commit/1a29e8675a64a5cdd2d93b6fa879a82d9a4d926a).
    - zstd supported by [docker >=v23.0.0](https://github.com/moby/moby/releases/tag/v23.0.0).
    - zstd supported by [containerd >= v1.5](https://github.com/containerd/containerd/releases/tag/v1.5.0).
  - `min-chunk-size`, `external-toc` (described in Tips 1) are unsupported yet.

```console
$ nerdctl build -t example.com/foo .
$ nerdctl image convert --zstdchunked --oci example.com/foo example.com/foo:zstdchunked
$ nerdctl push example.com/foo:zstdchunked
```
