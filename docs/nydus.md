# Lazy-pulling using Nydus Snapshotter

| :zap: Requirement | nerdctl >= 0.22 |
| ----------------- | --------------- |

Nydus snapshotter is a remote snapshotter plugin of containerd for [Nydus](https://github.com/dragonflyoss/image-service) image service which implements a chunk-based content-addressable filesystem that improves the current OCI image specification, in terms of container launching speed, image space, and network bandwidth efficiency, as well as data integrity with several runtime backends: FUSE, virtiofs and in-kernel EROFS (Linux kernel 5.19+).

## Enable lazy-pulling for `nerdctl run`

- Install containerd remote snapshotter plugin (`containerd-nydus-grpc`) from https://github.com/containerd/nydus-snapshotter

- Add the following to `/etc/containerd/config.toml`:
```toml
[proxy_plugins]
  [proxy_plugins.nydus]
    type = "snapshot"
    address = "/run/containerd-nydus-grpc/containerd-nydus-grpc.sock"
```

- Launch `containerd` and `containerd-nydus-grpc`

- Run `nerdctl` with `--snapshotter=nydus`
```console
# nerdctl --snapshotter=nydus run -it --rm ghcr.io/dragonflyoss/image-service/ubuntu:nydus-nightly-v5
```

For the list of pre-converted Nydus images, see https://github.com/orgs/dragonflyoss/packages?page=1&repo_name=image-service

For more details about how to build Nydus image, please refer to [nydusify](https://github.com/dragonflyoss/image-service/blob/master/docs/nydusify.md) conversion tool and [acceld](https://github.com/goharbor/acceleration-service).
