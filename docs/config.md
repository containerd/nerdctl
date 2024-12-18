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
hosts_dir      = ["/etc/containerd/certs.d", "/etc/docker/certs.d"]
experimental   = true
```

## Properties

| TOML property       | CLI flag                           | Env var                   | Description                                                                                                                                                      | Availability \*1 |
|---------------------|------------------------------------|---------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------|
| `debug`             | `--debug`                          |                           | Debug mode                                                                                                                                                       | Since 0.16.0     |
| `debug_full`        | `--debug-full`                     |                           | Debug mode (with full output)                                                                                                                                    | Since 0.16.0     |
| `address`           | `--address`,`--host`,`-a`,`-H`     | `$CONTAINERD_ADDRESS`     | containerd address                                                                                                                                               | Since 0.16.0     |
| `namespace`         | `--namespace`,`-n`                 | `$CONTAINERD_NAMESPACE`   | containerd namespace                                                                                                                                             | Since 0.16.0     |
| `snapshotter`       | `--snapshotter`,`--storage-driver` | `$CONTAINERD_SNAPSHOTTER` | containerd snapshotter                                                                                                                                           | Since 0.16.0     |
| `cni_path`          | `--cni-path`                       | `$CNI_PATH`               | CNI binary directory                                                                                                                                             | Since 0.16.0     |
| `cni_netconfpath`   | `--cni-netconfpath`                | `$NETCONFPATH`            | CNI config directory                                                                                                                                             | Since 0.16.0     |
| `data_root`         | `--data-root`                      |                           | Persistent state directory                                                                                                                                       | Since 0.16.0     |
| `cgroup_manager`    | `--cgroup-manager`                 |                           | cgroup manager                                                                                                                                                   | Since 0.16.0     |
| `insecure_registry` | `--insecure-registry`              |                           | Allow insecure registry                                                                                                                                          | Since 0.16.0     |
| `hosts_dir`         | `--hosts-dir`                      |                           | `certs.d` directory                                                                                                                                              | Since 0.16.0     |
| `experimental`      | `--experimental`                   | `NERDCTL_EXPERIMENTAL`    | Enable  [experimental features](experimental.md)                                                                                                                 | Since 0.22.3     |
| `host_gateway_ip`   | `--host-gateway-ip`                | `NERDCTL_HOST_GATEWAY_IP` | IP address that the special 'host-gateway' string in --add-host resolves to. Defaults to the IP address of the host. It has no effect without setting --add-host | Since 1.3.0      |
| `bridge_ip`         | `--bridge-ip`                      | `NERDCTL_BRIDGE_IP`       | IP address for the default nerdctl bridge network, e.g., 10.1.100.1/24                                                                                           | Since 2.0.1      |
| `kube-hide-dupe`    | `--kube-hide-dupe`                 |                           | Deduplicate images for Kubernetes with namespace k8s.io, no more redundant <none> ones are displayed    | Since 2.0.3      |

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
