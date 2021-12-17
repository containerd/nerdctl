# Configuring nerdctl with `nerdctl.toml`

| :zap: Requirement | nerdctl >= 0.16 |
|-------------------|-----------------|

This document describes the configuration file of nerdctl (`nerdctl.toml`).
This file is unrelated to the configuration file of containerd (`config.toml`) .

## File path
- Rootful mode:  `/etc/nerdctl/nerdctl.toml`
- Rootless mode: `~/.config/nerdctl/nerdctl.toml`

The path can be overridden with `$NERDCTL_TOML`.

## Example

```toml
# This is an example of /etc/nerdctl/nerdctl.toml .
# Unrelated to the daemon's /etc/containerd/config.toml .

debug          = false
debug_full     = false
address        = "unix:///run/k3s/containerd/containerd.sock"
namespace      = "k8s.io"
snapshotter    = "stargz"
cgroup_manager = "cgroupfs"
```

## Properties

| TOML property       | CLI flag                           | Env var                   | Description                   | Availability \*1 |
|---------------------|------------------------------------|---------------------------|-------------------------------|------------------|
| `debug`             | `--debug`                          |                           | Debug mode                    | Since 0.16.0     |
| `debug_full`        | `--debug-full`                     |                           | Debug mode (with full output) | Since 0.16.0     |
| `address`           | `--address`,`--host`,`-a`,`-H`     | `$CONTAINERD_ADDRESS`     | containerd address            | Since 0.16.0     |
| `namespace`         | `--namespace`,`-n`                 | `$CONTAINERD_NAMESPACE`   | containerd namespace          | Since 0.16.0     |
| `snapshotter`       | `--snapshotter`,`--storage-driver` | `$CONTAINERD_SNAPSHOTTER` | containerd snapshotter        | Since 0.16.0     |
| `cni_path`          | `--cni-path`                       | `$CNI_PATH`               | CNI binary directory          | Since 0.16.0     |
| `cni_netconfpath`   | `--cni-netconfpath`                | `$NETCONFPATH`            | CNI config directory          | Since 0.16.0     |
| `data_root`         | `--data-root`                      |                           | Persistent state directory    | Since 0.16.0     |
| `cgroup_manager`    | `--cgroup-manager`                 |                           | cgroup manager                | Since 0.16.0     |
| `insecure_registry` | `--insecure-registry`              |                           | Allow insecure registry       | Since 0.16.0     |

The properties are parsed in the following precedence:
1. CLI flag
2. Env var
3. TOML property
4. Built-in default value (Run `nerdctl --help` to see the default values)

\*1: Availability of the TOML properties

## See also
- [`registry.md`](registry.md)
- [`faq.md`](faq.md)
- https://github.com/containerd/containerd/blob/main/docs/ops.md#base-configuration (`/etc/containerd/config.toml`)
