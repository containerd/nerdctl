# Lazy-pulling using OverlayBD Snapshotter

| :zap: Requirement | nerdctl >= 0.15.0 |
| ----------------- | --------------- |

OverlayBD is a remote container image format base on block-device which is an open-source implementation of paper ["DADI: Block-Level Image Service for Agile and Elastic Application Deployment. USENIX ATC'20".](https://www.usenix.org/conference/atc20/presentation/li-huiba)

See https://github.com/containerd/accelerated-container-image to learn further information.

## Enable lazy-pulling for `nerdctl run`

- Install containerd remote snapshotter plugin (`overlaybd`) from https://github.com/containerd/accelerated-container-image/blob/main/docs/BUILDING.md

- Add the following to `/etc/containerd/config.toml`:
```toml
[proxy_plugins]
  [proxy_plugins.overlaybd]
    type = "snapshot"
    address = "/run/overlaybd-snapshotter/overlaybd.sock"
```

- Launch `containerd` and `overlaybd-snapshotter`

- Run `nerdctl` with `--snapshotter=overlaybd`
```console
nerdctl run --net host -it --rm --snapshotter=overlaybd registry.hub.docker.com/overlaybd/redis:6.2.1_obd
```

For more details about how to build overlaybd image, please refer to [accelerated-container-image](https://github.com/containerd/accelerated-container-image/blob/main/docs/IMAGE_CONVERTOR.md) conversion tool.