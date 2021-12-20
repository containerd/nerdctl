# FAQs and Troubleshooting

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Project](#project)
  - [How is nerdctl different from `docker` ?](#how-is-nerdctl-different-from-docker-)
  - [How is nerdctl different from `ctr` and `crictl` ?](#how-is-nerdctl-different-from-ctr-and-crictl-)
- [Mac & Windows](#mac--windows)
  - [Does nerdctl run on macOS ?](#does-nerdctl-run-on-macos-)
  - [Does nerdctl run on Windows ?](#does-nerdctl-run-on-windows-)
- [Configuration](#configuration)
  - [nerdctl ignores `[plugins."io.containerd.grpc.v1.cri"]` config](#nerdctl-ignores-pluginsiocontainerdgrpcv1cri-config)
  - [How to login to a registry?](#how-to-login-to-a-registry)
  - [How to use a non-HTTPS registry?](#how-to-use-a-non-https-registry)
  - [How to change the cgroup driver?](#how-to-change-the-cgroup-driver)
  - [How to change the snapshotter?](#how-to-change-the-snapshotter)
  - [How to change the runtime?](#how-to-change-the-runtime)
  - [How to change the CNI binary path?](#how-to-change-the-cni-binary-path)
- [Kubernetes](#kubernetes)
  - [`nerdctl ps -a` does not show Kubernetes containers](#nerdctl-ps--a-does-not-show-kubernetes-containers)
- [containerd socket (`/run/containerd/containerd.sock`)](#containerd-socket-runcontainerdcontainerdsock)
  - [Does nerdctl have an equivalent of `DOCKER_HOST=ssh://<USER>@<REMOTEHOST>` ?](#does-nerdctl-have-an-equivalent-of-docker_hostsshuserremotehost-)
  - [Does nerdctl have an equivalent of `sudo usermod -aG docker <USER>` ?](#does-nerdctl-have-an-equivalent-of-sudo-usermod--ag-docker-user-)
- [Rootless](#rootless)
  - [How to use nerdctl as a non-root user? (Rootless mode)](#how-to-use-nerdctl-as-a-non-root-user-rootless-mode)
  - [`nerdctl run -p <PORT>` does not propagate source IP](#nerdctl-run--p-port-does-not-propagate-source-ip)
  - [`nerdctl run -p <PORT>` does not work with port numbers below 1024](#nerdctl-run--p-port-does-not-work-with-port-numbers-below-1024)
  - [Can't ping](#cant-ping)
  - [Containers do not automatically start after rebooting the host](#containers-do-not-automatically-start-after-rebooting-the-host)
  - [Error `failed to create shim task: OCI runtime create failed: runc create failed: unable to start container process: unable to apply cgroup configuration: unable to start unit ... {Name:Slice Value:"user.slice"} {Name:Delegate Value:true} ... Permission denied: unknown`](#error-failed-to-create-shim-task-oci-runtime-create-failed-runc-create-failed-unable-to-start-container-process-unable-to-apply-cgroup-configuration-unable-to-start-unit--nameslice-valueuserslice-namedelegate-valuetrue--permission-denied-unknown)
  - [How to uninstall ? / Can't remove `~/.local/share/containerd`](#how-to-uninstall---cant-remove-localsharecontainerd)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Project

### How is nerdctl different from `docker` ?

The goal of nerdctl is to facilitate experimenting the cutting-edge features of containerd that are not present in Docker.

Such features include, but not limited to, [on-demand image pulling (lazy-pulling)](./stargz.md) and [image encryption/decryption](./ocicrypt.md).
See also [`../README.md`](../README.md) for the list of the features present in nerdctl but not present in Docker (and vice versa).

Note that competing with Docker is _not_ the goal of nerdctl. Those cutting-edge features are expected to be eventually available in Docker as well.

### How is nerdctl different from `ctr` and `crictl` ?

[`ctr`](https://github.com/containerd/containerd/tree/master/cmd/ctr) is a debugging utility bundled with containerd.

ctr is incompatible with Docker CLI, and not friendly to users.

Notably, `ctr` lacks the equivalents of the following nerdctl commands:
- `nerdctl run -p <PORT>`
- `nerdctl run --restart=always --net=bridge`
- `nerdctl pull` with `~/.docker/config.json` and credential helper binaries such as `docker-credential-ecr-login`
- `nerdctl logs`
- `nerdctl build`
- `nerdctl compose up`

[`crictl`](https://github.com/kubernetes-sigs/cri-tools) has similar restrictions too.

## Mac & Windows

### Does nerdctl run on macOS ?

Yes, via a Linux virtual machine.

[Lima](https://github.com/lima-vm/lima) project provides Linux virtual machines for macOS, with built-in integration for nerdctl.

```console
$ brew install lima
$ limactl start
$ lima nerdctl run -d --name nginx -p 127.0.0.1:8080:80 nginx:alpine
```

[Rancher Desktop for Mac](https://rancherdesktop.io/) and [colima](https://github.com/abiosoft/colima) also provide custom Lima machines with nerdctl.

### Does nerdctl run on Windows ?

Windows containers: Yes, but experiemental.

Linux containers: Yes, via WSL2. [Rancher Desktop for Windows](https://rancherdesktop.io/) provides a `nerdctl.exe` that wraps nerdctl binary in a WSL2 machine.

## Configuration

### nerdctl ignores `[plugins."io.containerd.grpc.v1.cri"]` config

Expected behavior, because nerdctl does not use CRI (Kubernetes Container Runtime Interface) API.

See the questions below for how to configure nerdctl.

### How to login to a registry?

Use `nerdctl login`, or just create `~/.docker/config.json`.
nerdctl also supports credential helper binaries such as `docker-credential-ecr-login`.

### How to use a non-HTTPS registry?

Use `nerdctl --insecure-registry run <IMAGE>`. See also [`registry.md`](./registry.md).

### How to change the cgroup driver?

- Option 1: `nerdctl --cgroup-manager=(cgroupfs|systemd|none)`.
- Option 2: Set `cgroup_manager` property in [`nerdctl.toml`](config.md)

The default value is `systemd` on cgroup v2 hosts (both rootful and rootless), `cgroupfs` on cgroup v1 rootful hosts, `none` on cgroup v1 rootless hosts.

<details>
<summary>Hint: The corresponding configuration for Kubernetes (<code>io.containerd.grpc.v1.cri</code>)</summary>

<p>

```toml
# An example of /etc/containerd/config.toml for Kubernetes
version = 2
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  SystemdCgroup = true
```

In addition to containerd, you have to configure kubelet too:

```yaml
# An example of /var/lib/kubelet/config.yaml for Kubernetes
kind: KubeletConfiguration
apiVersion: kubelet.config.k8s.io/v1beta1
cgroupDriver: "systemd"
```
See also https://kubernetes.io/docs/tasks/administer-cluster/kubeadm/configure-cgroup-driver/

</p>
</details>

### How to change the snapshotter?
- Option 1: Use `nerdctl --snapshotter=(overlayfs|native|btrfs|...)`
- Option 2: Set `$CONTAINERD_SNAPSHOTTER`
- Option 3: Set `snapshotter` property in [`nerdctl.toml`](config.md)

The default value is `overlayfs`.

<details>
<summary>Hint: The corresponding configuration for Kubernetes (<code>io.containerd.grpc.v1.cri</code>)</summary>

<p>

```toml
# An example of /etc/containerd/config.toml for Kubernetes
version = 2
[plugins."io.containerd.grpc.v1.cri".containerd]
  snapshotter = "overlayfs"
```

</p>
</details>

### How to change the runtime?
Use `nerdctl run --runtime=<RUNTIME>`.

The `<RUNTIME>` string can be either a containerd runtime plugin name (such as `io.containerd.runc.v2`),
or a path to a runc-compatible binary (such as `/usr/local/sbin/runc`).

<details>
<summary>Hint: The corresponding configuration for Kubernetes (<code>io.containerd.grpc.v1.cri</code>)</summary>

<p>

```toml
# An example of /etc/containerd/config.toml for Kubernetes
version = 2
[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "crun"
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
    [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.crun]
      runtime_type = "io.containerd.runc.v2"
      [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.crun.options]
        BinaryName = "/usr/local/bin/crun"
```

</p>
</details>


### How to change the CNI binary path?

- Option 1: Use `nerdctl --cni-path=<PATH>`
- Option 2: Set `$CNI_PATH`
- Option 3: Set `cni_path` property in [`nerdctl.toml`](config.md).

The default value is automatically detected by checking the following candidates:
- `~/.local/libexec/cni`
- `~/.local/lib/cni`
- `~/opt/cni/bin`
- `/usr/local/libexec/cni`
- `/usr/local/lib/cni`
- `/usr/libexec/cni`
- `/usr/lib/cni`
- `/opt/cni/bin`

<details>
<summary>Hint: The corresponding configuration for Kubernetes (<code>io.containerd.grpc.v1.cri</code>)</summary>

<p>

```toml
# An example of /etc/containerd/config.toml for Kubernetes
version = 2
[plugins."io.containerd.grpc.v1.cri".cni]
   bin_dir = "/opt/cni/bin"
```

</p>
</details>


## Kubernetes

### `nerdctl ps -a` does not show Kubernetes containers
Try `sudo nerdctl --namespace=k8s.io ps -a` .

Note: k3s users have to specify `--address` too: `sudo nerdctl --address=/run/k3s/containerd/containerd.sock --namespace=k8s.io ps -a`

## containerd socket (`/run/containerd/containerd.sock`)

### Does nerdctl have an equivalent of `DOCKER_HOST=ssh://<USER>@<REMOTEHOST>` ?

No. Just use `ssh -l <USER> <REMOTEHOST> nerdctl`.

### Does nerdctl have an equivalent of `sudo usermod -aG docker <USER>` ?

Not exactly same, but setting SETUID bit (`chmod +s`) on `nerdctl` binary gives similar user experience.

```
mkdir -p $HOME/bin
chmod 700 $HOME/bin
cp /usr/local/bin/nerdctl $HOME/bin
sudo chown root $HOME/bin/nerdctl
sudo chmod +s $HOME/bin/nerdctl
export PATH=$HOME/bin:$PATH
```

Chmodding `$HOME/bin` to `700` is important, otherwise an unintended user may gain the root privilege via the SETUID bit.

Using SETUID bit is highly discouraged. Consider using [Rootless mode](#rootless) instead whenever possible.

## Rootless

### How to use nerdctl as a non-root user? (Rootless mode)

```
containerd-rootless-setuptool.sh install
nerdctl run -d --name nginx -p 8080:80 nginx:alpine
```

See also:
- [`rootless.md`](./rootless.md)
- https://rootlesscontaine.rs/getting-started/common/
- https://rootlesscontaine.rs/getting-started/containerd/

### `nerdctl run -p <PORT>` does not propagate source IP
Expected behavior with the default `rootlesskit` port driver.

The solution is to change the port driver to `slirp4netns` (sacrifices performance).

See https://rootlesscontaine.rs/getting-started/containerd/#changing-the-port-forwarder .

### `nerdctl run -p <PORT>` does not work with port numbers below 1024

Set sysctl value `net.ipv4.ip_unprivileged_port_start=0` .

See https://rootlesscontaine.rs/getting-started/common/sysctl/#optional-allowing-listening-on-tcp--udp-ports-below-1024

### Can't ping

Set sysctl value `net.ipv4.ping_group_range=0 2147483647` .

See https://rootlesscontaine.rs/getting-started/common/sysctl/#optional-allowing-ping

### Containers do not automatically start after rebooting the host
Run `sudo loginctl enable-linger $(whoami)` .

See https://rootlesscontaine.rs/getting-started/common/login/ .

### Error `failed to create shim task: OCI runtime create failed: runc create failed: unable to start container process: unable to apply cgroup configuration: unable to start unit ... {Name:Slice Value:"user.slice"} {Name:Delegate Value:true} ... Permission denied: unknown`

Running a rootless container with `systemd` cgroup driver requires dbus to be running as a user session service.

Otherwise runc may fail with an error like below:
```
FATA[0000] failed to create shim task: OCI runtime create failed: runc create failed: unable to start container process: unable to apply cgroup configuration: unable to start unit "nerdctl-7bda4abaa1f006ab9feeb98c06953db43f212f1c0aaf658fb8a88d6f63dff9f9.scope" (properties [{Name:Description Value:"libcontainer container 7bda4abaa1f006ab9feeb98c06953db43f212f1c0aaf658fb8a88d6f63dff9f9"} {Name:Slice Value:"user.slice"} {Name:Delegate Value:true} {Name:PIDs Value:@au [1154]} {Name:MemoryAccounting Value:true} {Name:CPUAccounting Value:true} {Name:IOAccounting Value:true} {Name:TasksAccounting Value:true} {Name:DefaultDependencies Value:false}]): Permission denied: unknown
```

Solution:
```
sudo apt-get install -y dbus-user-session

systemctl --user start dbus
```

### How to uninstall ? / Can't remove `~/.local/share/containerd`

Run the following commands:
```
containerd-rootless-setuptool.sh uninstall
rootlesskit rm -rf ~/.local/share/containerd ~/.local/share/nerdctl ~/.config/containerd
```
