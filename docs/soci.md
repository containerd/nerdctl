# Lazy-pulling using SOCI Snapshotter

| :zap: Requirement | nerdctl >= 1.1.1 |
| ----------------- |------------------|

SOCI Snapshotter is a containerd snapshotter plugin. It enables standard OCI images to be lazily loaded without requiring a build-time conversion step. "SOCI" is short for "Seekable OCI", and is pronounced "so-CHEE".

See https://github.com/awslabs/soci-snapshotter to learn further information.

## Enable SOCI for `nerdctl run`

- Install containerd remote snapshotter plugin (`soci`) from https://github.com/awslabs/soci-snapshotter/blob/main/docs/GETTING_STARTED.md#prerequisites

- Add the following to `/etc/containerd/config.toml`:
```toml
[proxy_plugins]
  [proxy_plugins.soci]
    type = "snapshot"
    address = "/run/soci-snapshotter-grpc/soci-snapshotter-grpc.sock"
```

- Launch `containerd` and `soci-snapshotter`

- Run `nerdctl` with `--snapshotter=soci`
```console
nerdctl run --it --rm --snapshotter=soci docker.io/library/alpine:3.14.2
```

For more details about how to build soci image, please refer to [soci-snapshotter](https://github.com/awslabs/soci-snapshotter/blob/main/docs/GETTING_STARTED.md#create-and-push-a-soci-index)
