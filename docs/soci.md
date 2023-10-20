# Lazy-pulling using SOCI Snapshotter

SOCI Snapshotter is a containerd snapshotter plugin. It enables standard OCI images to be lazily loaded without requiring a build-time conversion step. "SOCI" is short for "Seekable OCI", and is pronounced "so-CHEE".

See https://github.com/awslabs/soci-snapshotter to learn further information.

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
