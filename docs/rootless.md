# Rootless mode

See https://rootlesscontaine.rs/getting-started/common/ for the prerequisites.

## Daemon (containerd)

Use [`containerd-rootless-setuptool.sh`](../extras/rootless) to set up rootless containerd.

```console
$ containerd-rootless-setuptool.sh install
[INFO] Checking RootlessKit functionality
[INFO] Checking cgroup v2
[INFO] Checking overlayfs
[INFO] Creating /home/testuser/.config/systemd/user/containerd.service
...
[INFO] Installed containerd.service successfully.
[INFO] To control containerd.service, run: `systemctl --user (start|stop|restart) containerd.service`
[INFO] To run containerd.service on system startup, run: `sudo loginctl enable-linger testuser`

[INFO] Use `nerdctl` to connect to the rootless containerd.
[INFO] You do NOT need to specify $CONTAINERD_ADDRESS explicitly.
```

The usage of `containerd-rootless-setuptool.sh` is almost same as [`dockerd-rootless-setuptool.sh`](https://rootlesscontaine.rs/getting-started/docker/) .

Resource limitation flags such as `nerdctl run --memory` require systemd and cgroup v2: https://rootlesscontaine.rs/getting-started/common/cgroup2/

## Client (nerdctl)

Just execute `nerdctl`. No need to specify the socket address manually.

```console
$ nerdctl run -it --rm alpine
```

Depending on your kernel version, you may need to enable FUSE-OverlayFS or set `export CONTAINERD_SNAPSHOTTER=native`.
(See below.)

## Add-ons
### BuildKit
To enable BuildKit, run the following command:

```console
$ containerd-rootless-setuptool.sh install-buildkit
```

## Snapshotters

### OverlayFS

The default `overlayfs` snapshotter only works on the following hosts:
- Any distro, with kernel >= 5.13
- Non-SELinux distro, with kernel >= 5.11
- Ubuntu since 2015

For other hosts, [`fuse-overlayfs` snapshotter](https://github.com/containerd/fuse-overlayfs-snapshotter) needs to be used instead.

### FUSE-OverlayFS

To enable `fuse-overlayfs` snapshotter, run the following command:
```console
$ containerd-rootless-setuptool.sh install-fuse-overlayfs
```

Then, add the following config to `~/.config/containerd/config.toml`, and run `systemctl --user restart containerd.service`:
```toml
[proxy_plugins]
  [proxy_plugins."fuse-overlayfs"]
      type = "snapshot"
# NOTE: replace "1000" with your actual UID
      address = "/run/user/1000/containerd-fuse-overlayfs.sock"
```

The snapshotter can be specified as `$CONTAINERD_SNAPSHOTTER`.
```console
$ export CONTAINERD_SNAPSHOTTER=fuse-overlayfs
$ nerdctl run -it --rm alpine
```

If `fuse-overlayfs` does not work, try `export CONTAINERD_SNAPSHOTTER=native`.

### Stargz Snapshotter
[Stargz Snapshotter](./stargz.md) enables lazy-pulling of images.

To enable Stargz snapshotter, run the following command:
```console
$ containerd-rootless-setuptool.sh install-stargz
```

Then, add the following config to `~/.config/containerd/config.toml` and run `systemctl --user restart containerd.service`:
```toml
[proxy_plugins]
  [proxy_plugins."stargz"]
      type = "snapshot"
# NOTE: replace "1000" with your actual UID
      address = "/run/user/1000/containerd-stargz-grpc/containerd-stargz-grpc.sock"
```

The snapshotter can be specified as `$CONTAINERD_SNAPSHOTTER`.
```console
$ export CONTAINERD_SNAPSHOTTER=stargz
$ nerdctl run -it --rm ghcr.io/stargz-containers/alpine:3.10.2-esgz
```

See https://github.com/containerd/stargz-snapshotter/blob/master/docs/pre-converted-images.md for the image list.

## Troubleshooting

### Hint to Fedora users
- If SELinux is enabled on your host and your kernel is older than 5.13, you need to use [`fuse-overlayfs` instead of `overlayfs`](#fuse-overlayfs).
