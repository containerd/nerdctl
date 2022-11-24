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

## Build OverlayBD image using `nerdctl image convert`

Nerdctl supports to convert an OCI image or docker format v2 image to OverlayBD image by using the `nerdctl image convert` command.

Before the conversion, you should have the `overlaybd-snapshotter` binary installed, which build from [accelerated-container-image](https://github.com/containerd/accelerated-container-image). You can run the command like `nerdctl image convert --overlaybd --oci <source_image> <target_image>` to convert the `<source_image>` to a OverlayBD image whose tag is `<target_image>`.
