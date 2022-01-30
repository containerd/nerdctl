# nerdctl directory layout

## Config
**Default**: `/etc/nerdctl/nerdctl.toml` (rootful), `~/.config/nerdctl/nerdctl.toml` (rootless)

The configuration file of nerdctl. See [`config.md`](./config.md).

Can be overridden with environment variable `$NERDCTL_TOML`.

This file is unrelated to the daemon config file `/etc/containerd/config.toml`.

## Data
### `<DATAROOT>`
**Default**: `/var/lib/nerdctl` (rootful), `~/.local/share/nerdctl` (rootless)

Can be overridden with `nerdctl --data-root=<DATAROOT>` flag.

The directory is solely managed by nerdctl, not by containerd.
The directory has nothing to do with containerd data root `/var/lib/containerd`.

### `<DATAROOT>/<ADDRHASH>`
e.g. `/var/lib/nerdctl/1935db59`

`1935db9` is from `$(echo -n "/run/containerd/containerd.sock" | sha256sum | cut -c1-8)`

This directory is also called "data store" in the implementation.

### `<DATAROOT>/<ADDRHASH>/containers/<NAMESPACE>/<CID>`
e.g. `/var/lib/nerdctl/1935db59/containers/default/c4ed811cc361d26faffdee8d696ddbc45a9d93c571b5b3c54d3da01cb29caeb1`

Files:
- `resolv.conf`: mounted to the container as `/etc/resolv.conf`
- `hostname`: mounted to the container as `/etc/hostname`
- `<CID>-json.log`: used by `nerdctl logs`
- `oci-hook.*.log`: logs of the OCI hook

### `<DATAROOT>/<ADDRHASH>/names/<NAMESPACE>`
e.g. `/var/lib/nerdctl/1935db59/names/default`

Files:
- `<NAME>`: contains the container ID (CID). Represents that the name is taken by that container. 

Files must be operated with a `LOCK_EX` lock against the `<DATAROOT>/<ADDRHASH>/names/<NAMESPACE>` directory.

### `<DATAROOT>/<ADDRHASH>/etchosts/<NAMESPACE>/<CID>`
e.g. `/var/lib/nerdctl/1935db59/etchosts/default/c4ed811cc361d26faffdee8d696ddbc45a9d93c571b5b3c54d3da01cb29caeb1`

Files:
- `hosts`: mounted to the container as `/etc/hosts`
- `meta.json`: metadata

Files must be operated with a `LOCK_EX` lock against the `<DATAROOT>/<ADDRHASH>/etchosts` directory.

### `<DATAROOT>/<ADDRHASH>/volumes/<NAMESPACE>/<VOLNAME>/_data`
e.g. `/var/lib/nerdctl/1935db59/volumes/default/foo/_data`

Data volume

## CNI

### `<NETCONFPATH>`
**Default**: `/etc/cni/net.d` (rootful), `~/.config/cni/net.d` (rootless)

Can be overridden with `nerdctl --cni-netconfpath=<NETCONFPATH>` flag and environment variable `$NETCONFPATH`.

Files:
- `nerdctl-<NWNAME>.conflist`: CNI conf list created by nerdctl
