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
[INFO] To run containerd.service on system startup, run: `sudo loginctl enable-linger suda`

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
