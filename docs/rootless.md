# Rootless mode

See https://rootlesscontaine.rs/getting-started/common/ for the prerequisites.

## Daemon (containerd)

Use [`containerd-rootless-setuptool.sh`)(../extras/rootless) to set up rootless containerd.

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

The script currently requires systemd and cgroup v2: https://rootlesscontaine.rs/getting-started/common/cgroup2/

## Client (nerdctl)

Just execute `nerdctl`. No need to specify the socket address manually.

```console
$ nerdctl run -it --rm alpine
```

Depending on your kernel version, you may need to set `export CONTAINERD_SNAPSHOTTER=native`.
See https://rootlesscontaine.rs/how-it-works/overlayfs/ .

## Troubleshooting

### Hint to Fedora 33 users

#### runc rpm
The runc package of Fedora 33 does not support cgroup v2.
Use the upstream runc binary: https://github.com/opencontainers/runc/releases

The runc package of Fedora 34 will probably support cgroup v2.

Alternatively, you may choose to use `crun` instead of `runc`:
`nerdctl run --runtime=crun`

#### OverlayFS
You need to set `export CONTAINERD_SNAPSHOTTER=native` on Fedora 33 because Fedora 33 does not support rootless overlayfs.
(FUSE-OverlayFS could be used instead, but you might need to recompile containerd, see https://github.com/AkihiroSuda/containerd-fuse-overlayfs)

Fedora 34 (kernel >= 5.11) will probably support rootless overlayfs.

#### SELinux
If SELinux is enabled on your host, probably you need the following workaround to avoid `can't open lock file /run/xtables.lock:` error:
```bash
sudo dnf install -y policycoreutils-python-utils
sudo semanage permissive -a iptables_t
```

See https://github.com/moby/moby/issues/41230 .
This workaround will no longer be needed after the release of Fedora 34.
