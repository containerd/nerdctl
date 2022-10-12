[[⬇️ **Download]**](https://github.com/containerd/nerdctl/releases)
[[📖 **Command reference]**](#command-reference)
[[❓**FAQs & Troubleshooting]**](./docs/faq.md)
[[📚 **Additional documents]**](#additional-documents)

# nerdctl: Docker-compatible CLI for containerd

`nerdctl` is a Docker-compatible CLI for [contai**nerd**](https://containerd.io).

 ✅ Same UI/UX as `docker`

 ✅ Supports Docker Compose (`nerdctl compose up`)

 ✅ [Optional] Supports [rootless mode, without slirp overhead (bypass4netns)](./docs/rootless.md)

 ✅ [Optional] Supports lazy-pulling ([Stargz](./docs/stargz.md), [Nydus](./docs/nydus.md), [OverlayBD](./docs/overlaybd.md))

 ✅ [Optional] Supports [encrypted images (ocicrypt)](./docs/ocicrypt.md)

 ✅ [Optional] Supports [P2P image distribution (IPFS)](./docs/ipfs.md) (\*1)

 ✅ [Optional] Supports [container image signing and verifying (cosign)](./docs/cosign.md)

nerdctl is a **non-core** sub-project of containerd.

\*1: P2P image distribution (IPFS) is completely optional. Your host is NOT connected to any P2P network, unless you opt in to [install and run IPFS daemon](https://docs.ipfs.io/install/).

## Examples

### Basic usage

To run a container with the default `bridge` CNI network (10.4.0.0/24):
```console
# nerdctl run -it --rm alpine
```

To build an image using BuildKit:
```console
# nerdctl build -t foo /some-dockerfile-directory
# nerdctl run -it --rm foo
```

To build and send output to a local directory using BuildKit:
```console
# nerdctl build -o type=local,dest=. /some-dockerfile-directory
```

To run containers from `docker-compose.yaml`:
```console
# nerdctl compose -f ./examples/compose-wordpress/docker-compose.yaml up
```

See also [`./examples/compose-wordpress`](./examples/compose-wordpress).

### Debugging Kubernetes

To list local Kubernetes containers:
```console
# nerdctl --namespace k8s.io ps -a
```

To build an image for local Kubernetes without using registry:
```console
# nerdctl --namespace k8s.io build -t foo /some-dockerfile-directory
# kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: foo
spec:
  containers:
    - name: foo
      image: foo
      imagePullPolicy: Never
EOF
```

To load an image archive (`docker save` format or OCI format) into local Kubernetes:
```console
# nerdctl --namespace k8s.io load < /path/to/image.tar
```

### Rootless mode

To launch rootless containerd:
```console
$ containerd-rootless-setuptool.sh install
```

To run a container with rootless containerd:
```console
$ nerdctl run -d -p 8080:80 --name nginx nginx:alpine
```

See [`./docs/rootless.md`](./docs/rootless.md).

## Install
Binaries are available here: https://github.com/containerd/nerdctl/releases

In addition to containerd, the following components should be installed:
- [CNI plugins](https://github.com/containernetworking/plugins): for using `nerdctl run`.
   - v1.1.0 or later is highly recommended. Older versions require extra [CNI isolation plugin](https://github.com/AkihiroSuda/cni-isolation) for isolating bridge networks (`nerdctl network create`).
- [BuildKit](https://github.com/moby/buildkit) (OPTIONAL): for using `nerdctl build`. BuildKit daemon (`buildkitd`) needs to be running. See also [the document about setting up BuildKit](./docs/build.md).
- [RootlessKit](https://github.com/rootless-containers/rootlesskit) and [slirp4netns](https://github.com/rootless-containers/slirp4netns) (OPTIONAL): for [Rootless mode](./docs/rootless.md)
   - RootlessKit needs to be v0.10.0 or later. v0.14.1 or later is recommended.
   - slirp4netns needs to be v0.4.0 or later. v1.1.7 or later is recommended.

These dependencies are included in `nerdctl-full-<VERSION>-<OS>-<ARCH>.tar.gz`, but not included in `nerdctl-<VERSION>-<OS>-<ARCH>.tar.gz`.

### Brew
On Linux systems you can install nerdctl via [brew](https://brew.sh):
```bash
brew install nerdctl
```
This is currently not supported for macOS. The section below shows how to install on macOS using brew.

### macOS

[Lima](https://github.com/lima-vm/lima) project provides Linux virtual machines for macOS, with built-in integration for nerdctl.

```console
$ brew install lima
$ limactl start
$ lima nerdctl run -d --name nginx -p 127.0.0.1:8080:80 nginx:alpine
```

### FreeBSD

See [`./docs/freebsd.md`](docs/freebsd.md).

### Windows

- Linux containers: Known to work on WSL2
- Windows containers: experimental support for Windows (see below for features that are currently known to work)

### Docker

To run containerd and nerdctl inside Docker:
```bash
docker build -t nerdctl .
docker run -it --rm --privileged nerdctl
```

## Motivation

The goal of `nerdctl` is to facilitate experimenting the cutting-edge features of containerd that are not present in Docker (see below).

Note that competing with Docker is _not_ the goal of `nerdctl`. Those cutting-edge features are expected to be eventually available in Docker as well.

Also, `nerdctl` might be potentially useful for debugging Kubernetes clusters, but it is not the primary goal.

## Features present in `nerdctl` but not present in Docker
Major:
- On-demand image pulling (lazy-pulling) using [Stargz](./docs/stargz.md)/[Nydus](./docs/nydus.md)/[OverlayBD](./docs/overlaybd.md) Snapshotter: `nerdctl --snapshotter=stargz|nydus|overlaybd run IMAGE` .
- [Image encryption and decryption using ocicrypt (imgcrypt)](./docs/ocicrypt.md): `nerdctl image (encrypt|decrypt) SRC DST`
- [P2P image distribution using IPFS](./docs/ipfs.md): `nerdctl run ipfs://CID` .
  P2P image distribution (IPFS) is completely optional. Your host is NOT connected to any P2P network, unless you opt in to [install and run IPFS daemon](https://docs.ipfs.io/install/).
- Recursive read-only (RRO) bind-mount: `nerdctl run -v /mnt:/mnt:rro` (make children such as `/mnt/usb` to be read-only, too).
  Requires kernel >= 5.12, and crun >= 1.4 or runc >= 1.1 (PR [#3272](https://github.com/opencontainers/runc/pull/3272)).
- [Cosign integration](./docs/cosign.md): `nerdctl pull --verify=cosign` and `nerdctl push --sign=cosign`
- [Accelerated rootless containers using bypass4netns](./docs/rootless.md): `nerdctl run --label nerdctl/bypass4netns=true`

Minor:
- Namespacing: `nerdctl --namespace=<NS> ps` .
  (NOTE: All Kubernetes containers are in the `k8s.io` containerd namespace regardless to Kubernetes namespaces)
- Exporting Docker/OCI dual-format archives: `nerdctl save` .
- Importing OCI archives as well as Docker archives: `nerdctl load` .
- Specifying a non-image rootfs: `nerdctl run -it --rootfs <ROOTFS> /bin/sh` . The CLI syntax conforms to Podman convention.
- Connecting a container to multiple networks at once: `nerdctl run --net foo --net bar`
- Running [FreeBSD jails](./docs/freebsd.md).
- Better multi-platform support, e.g., `nerdctl pull --all-platforms IMAGE`
- Applying an (existing) AppArmor profile to rootless containers: `nerdctl run --security-opt apparmor=<PROFILE>`.
  Use `sudo nerdctl apparmor load` to load the `nerdctl-default` profile.

Trivial:
- Inspecting raw OCI config: `nerdctl container inspect --mode=native` .

## Similar tools

- [`ctr`](https://github.com/containerd/containerd/tree/master/cmd/ctr): incompatible with Docker CLI, and not friendly to users.
  Notably, `ctr` lacks the equivalents of the following nerdctl commands:
  - `nerdctl run -p <PORT>`
  - `nerdctl run --restart=always --net=bridge`
  - `nerdctl pull` with `~/.docker/config.json` and credential helper binaries such as `docker-credential-ecr-login`
  - `nerdctl logs`
  - `nerdctl build`
  - `nerdctl compose up`

- [`crictl`](https://github.com/kubernetes-sigs/cri-tools): incompatible with Docker CLI, not friendly to users, and does not support non-CRI features
- [k3c v0.2 (abandoned)](https://github.com/rancher/k3c/tree/v0.2.1): needs an extra daemon, and does not support non-CRI features
- [Rancher Kim (nee k3c v0.3)](https://github.com/rancher/kim): needs Kubernetes, and only focuses on image management commands such as `kim build` and `kim push`
- [PouchContainer (abandoned?)](https://github.com/alibaba/pouch): needs an extra daemon

## Developer guide
nerdctl is a containerd **non-core** sub-project, licensed under the [Apache 2.0 license](./LICENSE).
As a containerd non-core sub-project, you will find the:
 * [Project governance](https://github.com/containerd/project/blob/master/GOVERNANCE.md),
 * [Maintainers](./MAINTAINERS),
 * and [Contributing guidelines](https://github.com/containerd/project/blob/master/CONTRIBUTING.md)

information in our [`containerd/project`](https://github.com/containerd/project) repository.

### Compiling nerdctl from source

**Tips: You should make your Go toolchain newer than 1.17.**

Run `make && sudo make install`.

Using `go install github.com/containerd/nerdctl/cmd/nerdctl` is possible, but unrecommended because it does not fill version strings printed in `nerdctl version`

### Test suite
#### Running unit tests
Run `go test -v ./pkg/...`

#### Running integration test suite against nerdctl
Run `go test -exec sudo -v ./cmd/nerdctl/...` after `make && sudo make install`.

For testing rootless mode, `-exec sudo` is not needed.

To run tests in a container:
```bash
docker build -t test-integration --target test-integration .
docker run -t --rm --privileged test-integration
```
#### Running integration test suite against Docker
Run `go test -exec sudo -v ./cmd/nerdctl/... -args -test.target=docker` to ensure that the test suite is compatible with Docker.

### Contributing to nerdctl

Lots of commands and flags are currently missing. Pull requests are highly welcome.

Please certify your [Developer Certificate of Origin (DCO)](https://developercertificate.org/), by signing off your commit with `git commit -s` and with your real name.

- - -

# Command reference

:whale:     = Docker compatible

:nerd_face: = nerdctl specific

:blue_square: = Windows enabled

Unlisted `docker` CLI flags are unimplemented yet in `nerdctl` CLI.
It does not necessarily mean that the corresponding features are missing in containerd.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

  - [Container management](#container-management)
    - [:whale: :blue_square: nerdctl run](#whale-blue_square-nerdctl-run)
    - [:whale: :blue_square: nerdctl exec](#whale-blue_square-nerdctl-exec)
    - [:whale: :blue_square: nerdctl create](#whale-blue_square-nerdctl-create)
    - [:whale: nerdctl cp](#whale-nerdctl-cp)
    - [:whale: :blue_square: nerdctl ps](#whale-blue_square-nerdctl-ps)
    - [:whale: :blue_square: nerdctl inspect](#whale-blue_square-nerdctl-inspect)
    - [:whale: nerdctl logs](#whale-nerdctl-logs)
    - [:whale: nerdctl port](#whale-nerdctl-port)
    - [:whale: nerdctl rm](#whale-nerdctl-rm)
    - [:whale: nerdctl stop](#whale-nerdctl-stop)
    - [:whale: nerdctl start](#whale-nerdctl-start)
    - [:whale: nerdctl restart](#whale-nerdctl-restart)
    - [:whale: nerdctl update](#whale-nerdctl-update)
    - [:whale: nerdctl wait](#whale-nerdctl-wait)
    - [:whale: nerdctl kill](#whale-nerdctl-kill)
    - [:whale: nerdctl pause](#whale-nerdctl-pause)
    - [:whale: nerdctl unpause](#whale-nerdctl-unpause)
    - [:whale: docker rename](#whale-docker-rename)
    - [:whale: nerdctl container prune](#whale-nerdctl-container-prune)
  - [Build](#build)
    - [:whale: nerdctl build](#whale-nerdctl-build)
    - [:whale: nerdctl commit](#whale-nerdctl-commit)
  - [Image management](#image-management)
    - [:whale: :blue_square: nerdctl images](#whale-blue_square-nerdctl-images)
    - [:whale: :blue_square: nerdctl pull](#whale-blue_square-nerdctl-pull)
    - [:whale: nerdctl push](#whale-nerdctl-push)
    - [:whale: nerdctl load](#whale-nerdctl-load)
    - [:whale: nerdctl save](#whale-nerdctl-save)
    - [:whale: nerdctl tag](#whale-nerdctl-tag)
    - [:whale: nerdctl rmi](#whale-nerdctl-rmi)
    - [:whale: nerdctl image inspect](#whale-nerdctl-image-inspect)
    - [:whale: nerdctl image history](#whale-nerdctl-image-history)
    - [:whale: nerdctl image prune](#whale-nerdctl-image-prune)
    - [:nerd_face: nerdctl image convert](#nerd_face-nerdctl-image-convert)
    - [:nerd_face: nerdctl image encrypt](#nerd_face-nerdctl-image-encrypt)
    - [:nerd_face: nerdctl image decrypt](#nerd_face-nerdctl-image-decrypt)
  - [Registry](#registry)
    - [:whale: nerdctl login](#whale-nerdctl-login)
    - [:whale: nerdctl logout](#whale-nerdctl-logout)
  - [Network management](#network-management)
    - [:whale: nerdctl network create](#whale-nerdctl-network-create)
    - [:whale: nerdctl network ls](#whale-nerdctl-network-ls)
    - [:whale: nerdctl network inspect](#whale-nerdctl-network-inspect)
    - [:whale: nerdctl network rm](#whale-nerdctl-network-rm)
    - [:whale: nerdctl network prune](#whale-nerdctl-network-prune)
  - [Volume management](#volume-management)
    - [:whale: nerdctl volume create](#whale-nerdctl-volume-create)
    - [:whale: nerdctl volume ls](#whale-nerdctl-volume-ls)
    - [:whale: nerdctl volume inspect](#whale-nerdctl-volume-inspect)
    - [:whale: nerdctl volume rm](#whale-nerdctl-volume-rm)
    - [:whale: nerdctl volume prune](#whale-nerdctl-volume-prune)
  - [Namespace management](#namespace-management)
    - [:nerd_face: :blue_square: nerdctl namespace create](#nerd_face-blue_square-nerdctl-namespace-create)
    - [:nerd_face: :blue_square: nerdctl namespace inspect](#nerd_face-blue_square-nerdctl-namespace-inspect)
    - [:nerd_face: :blue_square: nerdctl namespace ls](#nerd_face-blue_square-nerdctl-namespace-ls)
    - [:nerd_face: :blue_square: nerdctl namespace remove](#nerd_face-blue_square-nerdctl-namespace-remove)
    - [:nerd_face: :blue_square: nerdctl namespace update](#nerd_face-blue_square-nerdctl-namespace-update)
  - [AppArmor profile management](#apparmor-profile-management)
    - [:nerd_face: nerdctl apparmor inspect](#nerd_face-nerdctl-apparmor-inspect)
    - [:nerd_face: nerdctl apparmor load](#nerd_face-nerdctl-apparmor-load)
    - [:nerd_face: nerdctl apparmor ls](#nerd_face-nerdctl-apparmor-ls)
    - [:nerd_face: nerdctl apparmor unload](#nerd_face-nerdctl-apparmor-unload)
  - [Builder management](#builder-management)
    - [:whale: nerdctl builder prune](#whale-nerdctl-builder-prune)
    - [:nerd_face: nerdctl builder debug](#nerd_face-nerdctl-builder-debug)
  - [System](#system)
    - [:whale: nerdctl events](#whale-nerdctl-events)
    - [:whale: nerdctl info](#whale-nerdctl-info)
    - [:whale: nerdctl version](#whale-nerdctl-version)
    - [:whale: nerdctl system prune](#whale-nerdctl-system-prune)
  - [Stats](#stats)
    - [:whale: nerdctl stats](#whale-nerdctl-stats)
    - [:whale: nerdctl top](#whale-nerdctl-top)
  - [Shell completion](#shell-completion)
    - [:nerd_face: nerdctl completion bash](#nerd_face-nerdctl-completion-bash)
    - [:nerd_face: nerdctl completion zsh](#nerd_face-nerdctl-completion-zsh)
    - [:nerd_face: nerdctl completion fish](#nerd_face-nerdctl-completion-fish)
    - [:nerd_face: nerdctl completion powershell](#nerd_face-nerdctl-completion-powershell)
  - [Compose](#compose)
    - [:whale: nerdctl compose](#whale-nerdctl-compose)
    - [:whale: nerdctl compose up](#whale-nerdctl-compose-up)
    - [:whale: nerdctl compose logs](#whale-nerdctl-compose-logs)
    - [:whale: nerdctl compose build](#whale-nerdctl-compose-build)
    - [:whale: nerdctl compose down](#whale-nerdctl-compose-down)
    - [:whale: nerdctl compose ps](#whale-nerdctl-compose-ps)
    - [:whale: nerdctl compose pull](#whale-nerdctl-compose-pull)
    - [:whale: nerdctl compose push](#whale-nerdctl-compose-push)
    - [:whale: nerdctl compose config](#whale-nerdctl-compose-config)
    - [:whale: nerdctl compose kill](#whale-nerdctl-compose-kill)
    - [:whale: nerdctl compose run](#whale-nerdctl-compose-run)
    - [:whale: nerdctl compose version](#whale-nerdctl-compose-version)
  - [IPFS management](#ipfs-management)
    - [:nerd_face: nerdctl ipfs registry up](#nerd_face-nerdctl-ipfs-registry-up)
    - [:nerd_face: nerdctl ipfs registry down](#nerd_face-nerdctl-ipfs-registry-down)
    - [:nerd_face: nerdctl ipfs registry serve](#nerd_face-nerdctl-ipfs-registry-serve)
  - [Global flags](#global-flags)
  - [Unimplemented Docker commands](#unimplemented-docker-commands)
- [Additional documents](#additional-documents)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->



## Container management
### :whale: :blue_square: nerdctl run
Run a command in a new container.

Usage: `nerdctl run [OPTIONS] IMAGE [COMMAND] [ARG...]`

:nerd_face: `ipfs://` prefix can be used for `IMAGE` to pull it from IPFS. See [`/docs/ipfs.md`](./docs/ipfs.md) for details.

Basic flags:
- :whale: :blue_square: `-i, --interactive`: Keep STDIN open even if not attached"
- :whale: :blue_square: `-t, --tty`: Allocate a pseudo-TTY
  - :warning: WIP: currently `-t` conflicts with `-d`
- :whale: :blue_square: `-d, --detach`: Run container in background and print container ID
- :whale: `--restart=(no|always|on-failure|unless-stopped)`: Restart policy to apply when a container exits
  - Default: "no"
  - always: Always restart the container if it stops.
  - on-failure[:max-retries]: Restart only if the container exits with a non-zero exit status. Optionally, limit the number of times attempts to restart the container using the :max-retries option.
  - unless-stopped: Always restart the container unless it is stopped.
- :whale: `--rm`: Automatically remove the container when it exits
- :whale: `--pull=(always|missing|never)`: Pull image before running
  - Default: "missing"
- :whale: `--pid=(host)`: PID namespace to use
- :whale: `--stop-signal`: Signal to stop a container (default "SIGTERM")
- :whale: `--stop-timeout`: Timeout (in seconds) to stop a container

Platform flags:
- :whale: `--platform=(amd64|arm64|...)`: Set platform

Init process flags:
- :whale: `--init`: Run an init inside the container that forwards signals and reaps processes.
- :nerd_face: `--init-binary=<binary-name>`: The custom init binary to use. We suggest you use the [tini](https://github.com/krallin/tini) binary which is used in Docker project to get the same behavior.
  Please make sure the binary exists in your `PATH`.
  - Default: `tini`

Network flags:
- :whale: `--net, --network=(bridge|host|none|container:<container>|<CNI>)`: Connect a container to a network.
  - Default: "bridge"
  - 'container:<name|id>': reuse another container's network stack, container has to be precreated.
  - :nerd_face: Unlike Docker, this flag can be specified multiple times (`--net foo --net bar`)
- :whale: `-p, --publish`: Publish a container's port(s) to the host
- :whale: `--dns`: Set custom DNS servers
- :whale: `--dns-search`: Set custom DNS search domains
- :whale: `--dns-opt, --dns-option`: Set DNS options
- :whale: `-h, --hostname`: Container host name
- :whale: `--add-host`: Add a custom host-to-IP mapping (host:ip)
- :whale: `--ip`: Specific static IP address(es) to use
- :whale: `--mac-address`: Specific MAC address to use. Be aware that it does not
  check if manually specified MAC addresses are unique. Supports network
  type `bridge` and `macvlan`

Resource flags:
- :whale: `--cpus`: Number of CPUs
- :whale: `--cpu-quota`: Limit the CPU CFS (Completely Fair Scheduler) quota
- :whale: `--cpu-period`: Limit the CPU CFS (Completely Fair Scheduler) period
- :whale: `--cpu-shares`: CPU shares (relative weight)
- :whale: `--cpuset-cpus`: CPUs in which to allow execution (0-3, 0,1)
- :whale: `--cpuset-mems`: Memory nodes (MEMs) in which to allow execution (0-3, 0,1). Only effective on NUMA systems
- :whale: `--memory`: Memory limit
- :whale: `--memory-reservation`: Memory soft limit
- :whale: `--memory-swap`: Swap limit equal to memory plus swap: '-1' to enable unlimited swap
- :whale: `--memory-swappiness`: Tune container memory swappiness (0 to 100) (default -1)
- :whale: `--kernel-memory`: Kernel memory limit (deprecated)
- :whale: `--oom-kill-disable`: Disable OOM Killer
- :whale: `--oom-score-adj`: Tune container’s OOM preferences (-1000 to 1000)
- :whale: `--pids-limit`: Tune container pids limit
- :nerd_face: `--cgroup-conf`: Configure cgroup v2 (key=value)
- :whale: `--blkio-weight`: Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)
- :whale: `--cgroupns=(host|private)`: Cgroup namespace to use
  - Default: "private" on cgroup v2 hosts, "host" on cgroup v1 hosts
- :whale: `--device`: Add a host device to the container

Intel RDT flags:
- :nerd_face: `--rdt-class=CLASS`: Name of the RDT class (or CLOS) to associate the container wit

User flags:
- :whale: :blue_square: `-u, --user`: Username or UID (format: <name|uid>[:<group|gid>])
- :nerd_face: `--umask`: Set the umask inside the container. Defaults to 0022.
  Corresponds to Podman CLI.
- :whale: `--group-add`: Add additional groups to join

Security flags:
- :whale: `--security-opt seccomp=<PROFILE_JSON_FILE>`: specify custom seccomp profile
- :whale: `--security-opt apparmor=<PROFILE>`: specify custom AppArmor profile
- :whale: `--security-opt no-new-privileges`: disallow privilege escalation, e.g., setuid and file capabilities
- :nerd_face: `--security-opt privileged-without-host-devices`: Don't pass host devices to privileged containers
- :whale: `--cap-add=<CAP>`: Add Linux capabilities
- :whale: `--cap-drop=<CAP>`: Drop Linux capabilities
- :whale: `--privileged`: Give extended privileges to this container

Runtime flags:
- :whale: `--runtime`: Runtime to use for this container, e.g. \"crun\", or \"io.containerd.runsc.v1\".
- :whale: `--sysctl`: Sysctl options, e.g \"net.ipv4.ip_forward=1\"

Volume flags:
- :whale: :blue_square: `-v, --volume <SRC>:<DST>[:<OPT>]`: Bind mount a volume, e.g., `-v /mnt:/mnt:rro,rprivate`
  - :whale:     option `rw` : Read/Write (when writable)
  - :whale:     option `ro` : Non-recursive read-only
  - :nerd_face: option `rro`: Recursive read-only. Should be used in conjunction with `rprivate`. e.g., `-v /mnt:/mnt:rro,rprivate` makes children such as `/mnt/usb` to be read-only, too.
    Requires kernel >= 5.12, and crun >= 1.4 or runc >= 1.1 (PR [#3272](https://github.com/opencontainers/runc/pull/3272)). With older runc, `rro` just works as `ro`.
  - :whale:     option `shared`, `slave`, `private`: Non-recursive "shared" / "slave" / "private" propagation
  - :whale:     option `rshared`, `rslave`, `rprivate`: Recursive "shared" / "slave" / "private" propagation
  - :nerd_face: option `bind`: Not-recursively bind-mounted
  - :nerd_face: option `rbind`: Recursively bind-mounted
- :whale: `--tmpfs`: Mount a tmpfs directory, e.g. `--tmpfs /tmp:size=64m,exec`.
- :whale: `--mount`: Attach a filesystem mount to the container.
  Consists of multiple key-value pairs, separated by commas and each
  consisting of a `<key>=<value>` tuple.
  e.g., `-- mount type=bind,source=/src,target=/app,bind-propagation=shared`.
  - :whale: `type`: Current supported mount types are `bind`, `volume`, `tmpfs`.
    The defaul type will be set to `volume` if not specified.
    i.e., `--mount src=vol-1,dst=/app,readonly` equals `--mount type=volum,src=vol-1,dst=/app,readonly`
  - Common Options:
    - :whale: `src`, `source`: Mount source spec for bind and volume. Mandatory for bind.
    - :whale: `dst`, `destination`, `target`: Mount destination spec.
    - :whale: `readonly`, `ro`, `rw`, `rro`: Filesystem permissinos.
  - Options specific to `bind`:
    - :whale: `bind-propagation`: `shared`, `slave`, `private`, `rshared`, `rslave`, or `rprivate`(default).
    - :whale: `bind-nonrecursive`: `true` or `false`(default). If set to true, submounts are not recursively bind-mounted. This option is useful for readonly bind mount.
    - unimplemented options: `consistency`
  - Options specific to `tmpfs`:
    - :whale: `tmpfs-size`: Size of the tmpfs mount in bytes. Unlimited by default.
    - :whale: `tmpfs-mode`: File mode of the tmpfs in **octal**.
      Defaults to `1777` or world-writable.
  - Options specific to `volume`:
    - unimplemented options: `volume-nocopy`, `volume-label`, `volume-driver`, `volume-opt`

Rootfs flags:
- :whale: `--read-only`: Mount the container's root filesystem as read only
- :nerd_face: `--rootfs`: The first argument is not an image but the rootfs to the exploded container.
  Corresponds to Podman CLI.

Env flags:
- :whale: :blue_square: `--entrypoint`: Overwrite the default ENTRYPOINT of the image
- :whale: :blue_square: `-w, --workdir`: Working directory inside the container
- :whale: :blue_square: `-e, --env`: Set environment variables
- :whale: :blue_square: `--env-file`: Set environment variables from file

Metadata flags:
- :whale: :blue_square: `--name`: Assign a name to the container
- :whale: :blue_square: `-l, --label`: Set meta data on a container
- :whale: :blue_square: `--label-file`: Read in a line delimited file of labels
- :whale: :blue_square: `--cidfile`: Write the container ID to the file
- :nerd_face: `--pidfile`: file path to write the task's pid. The CLI syntax conforms to Podman convention.

Logging flags:
- :whale: `--log-driver=(json-file|journald|fluentd|syslog)`: Logging driver for the container (default `json-file`).
    - :whale: `--log-driver=json-file`: The logs are formatted as JSON. The default logging driver for nerdctl.
      - The `json-file` logging driver supports the following logging options:
        - :whale: `--log-opt=max-size=<MAX-SIZE>`: The maximum size of the log before it is rolled. A positive integer plus a modifier representing the unit of measure (k, m, or g). Defaults to unlimited.
        - :whale: `--log-opt=max-file=<MAX-FILE>`: The maximum number of log files that can be present. If rolling the logs creates excess files, the oldest file is removed. Only effective when `max-size` is also set. A positive integer. Defaults to 1.
        - :nerd_face: `--log-opt=log-path=<LOG-PATH>`: The log path where the logs are written. The path will be created if it does not exist. If the log file exists, the old file will be renamed to `<LOG-PATH>.1`.
          - Default: `<data-root>/<containerd-socket-hash>/<namespace>/<container-id>/<container-id>-json.log`
          - Example: `/var/lib/nerdctl/1935db59/containers/default/<container-id>/<container-id>-json.log`
    - :whale: `--log-driver=journald`: Writes log messages to `journald`. The `journald` daemon must be running on the host machine.
      - :whale: `--log-opt=tag=<TEMPLATE>`: Specify template to set `SYSLOG_IDENTIFIER` value in journald logs.
    - :whale: `--log-driver=fluentd`: Writes log messages to `fluentd`. The `fluentd` daemon must be running on the host machine.
      - The `fluentd` logging driver supports the following logging options:
        - :whale: `--log-opt=fluentd-address=<ADDRESS>`: The address of the `fluentd` daemon, tcp(default) and unix sockets are supported..
        - :whale: `--log-opt=fluentd-async=<true|false>`: Enable async mode for fluentd. The default value is false.
        - :whale: `--log-opt=fluentd-buffer-limit=<LIMIT>`: The buffer limit for fluentd. If the buffer is full, the call to record logs will fail. The default is 8192. (https://github.com/fluent/fluent-logger-golang/tree/master#bufferlimit)
        - :whale: `--log-opt=fluentd-retry-wait=<1s|1ms>`: The time to wait before retrying to send logs to fluentd. The default value is 1s.
        - :whale: `--log-opt=fluentd-max-retries=<1>`: The maximum number of retries to send logs to fluentd. The default value is MaxInt32.
        - :whale: `--log-opt=fluentd-sub-second-precision=<true|false>`: Enable sub-second precision for fluentd. The default value is false.
        - :nerd_face: `--log-opt=fluentd-async-reconnect-interval=<1s|1ms>`: The time to wait before retrying to reconnect to fluentd. The default value is 0s.
        - :nerd_face: `--log-opt=fluentd-request-ack=<true|false>`: Enable request ack for fluentd. The default value is false.
    - :whale: `--log-driver=syslog`: Writes log messages to `syslog`. The
      `syslog` daemon must be running on either the host machine or remote.
      - The `syslog` logging driver supports the following logging options:
        - :whale: `--log-opt=syslog-address=<ADDRESS>`: The address of an
          external `syslog` server. The URI specifier may be
          `tcp|udp|tcp+tls]://host:port`, `unix://path`, or `unixgram://path`.
          If the transport is `tcp`, `udp`, or `tcp+tls`, the default port is
          `514`.
        - :whale: `--log-opt=syslog-facility=<FACILITY>`: The `syslog` facility to
          use. Can be the number or name for any valid syslog facility. See the
          [syslog documentation](https://www.rfc-editor.org/rfc/rfc5424#section-6.2.1).
        - :whale: `--log-opt=syslog-tls-ca-cert=<VALUE>`: The absolute path to
          the trust certificates signed by the CA. **Ignored if the address
          protocol is not `tcp+tls`**.
        - :whale: `--log-opt=syslog-tls-cert=<VALUE>`: The absolute path to
          the TLS certificate file. **Ignored if the address protocol is not
          `tcp+tls`**.
        - :whale: `--log-opt=syslog-tls-key=<VALUE>`:The absolute path to
          the TLS key file. **Ignored if the address protocol is not `tcp+tls`**.
        - :whale: `--log-opt=syslog-tls-skip-verify=<VALUE>`: If set to `true`,
          TLS verification is skipped when connecting to the daemon.
          **Ignored if the address protocol is not `tcp+tls`**.
        - :whale: `--log-opt=syslog-format=<VALUE>`: The `syslog` message format
          to use. If not specified the local UNIX syslog format is used,
          without a specified hostname. Specify `rfc3164` for the RFC-3164
          compatible format, `rfc5424` for RFC-5424 compatible format, or
          `rfc5424micro` for RFC-5424 compatible format with microsecond
          timestamp resolution.
        - :whale: `--log-opt=tag=<VALUE>`: A string that is appended to the
          `APP-NAME` in the `syslog` message. By default, nerdctl uses the first
          12 characters of the container ID to tag log messages.
    - :nerd_face: Accepts a LogURI which is a containerd shim logger. A scheme must be specified for the URI. Example: `nerdctl run -d --log-driver binary:///usr/bin/ctr-journald-shim docker.io/library/hello-world:latest`. An implementation of shim logger can be found at (https://github.com/containerd/containerd/tree/dbef1d56d7ebc05bc4553d72c419ed5ce025b05d/runtime/v2#logging)


Shared memory flags:
- :whale: `--ipc`: IPC namespace to use
- :whale: `--shm-size`: Size of `/dev/shm`

GPU flags:
- :whale: `--gpus`: GPU devices to add to the container ('all' to pass all GPUs). Please see also [./docs/gpu.md](./docs/gpu.md) for details.

Ulimit flags:
- :whale: `--ulimit`: Set ulimit

Verify flags:
- :nerd_face: `--verify`: Verify the image (none|cosign). See [`docs/cosign.md`](./docs/cosign.md) for details.
- :nerd_face: `--cosign-key`: Path to the public key file, KMS, URI or Kubernetes Secret for `--verify=cosign`

Unimplemented `docker run` flags:
    `--attach`, `--blkio-weight-device`, `--cgroup-parent`, `--cpu-rt-*`, `--detach-keys`, `--device-*`,
    `--disable-content-trust`, `--domainname`, `--expose`, `--health-*`, `--ip6`, `--isolation`, `--no-healthcheck`,
    `--link*`, `--mac-address`, `--publish-all`, `--sig-proxy`, `--storage-opt`,
    `--userns`, `--uts`, `--volume-driver`, `--volumes-from`

### :whale: :blue_square: nerdctl exec
Run a command in a running container.

Usage: `nerdctl exec [OPTIONS] CONTAINER COMMAND [ARG...]`

Flags:
- :whale: `-i, --interactive`: Keep STDIN open even if not attached
- :whale: `-t, --tty`: Allocate a pseudo-TTY
  - :warning: WIP: currently `-t` conflicts with `-d`
- :whale: `-d, --detach`: Detached mode: run command in the background
- :whale: `-w, --workdir`: Working directory inside the container
- :whale: `-e, --env`: Set environment variables
- :whale: `--env-file`: Set environment variables from file
- :whale: `--privileged`: Give extended privileges to the command
- :whale: `-u, --user`: Username or UID (format: <name|uid>[:<group|gid>])

Unimplemented `docker exec` flags: `--detach-keys`


### :whale: :blue_square: nerdctl create
Create a new container.

Usage: `nerdctl create [OPTIONS] IMAGE [COMMAND] [ARG...]`

:nerd_face: `ipfs://` prefix can be used for `IMAGE` to pull it from IPFS. See [`/docs/ipfs.md`](./docs/ipfs.md) for details.

The `nerdctl create` command similar to `nerdctl run -d` except the container is never started. You can then use the `nerdctl start <container_id>` command to start the container at any point.

### :whale: nerdctl cp
Copy files/folders between a running container and the local filesystem

Usage:
- `nerdctl cp [OPTIONS] CONTAINER:SRC_PATH DEST_PATH|-`
- `nerdctl cp [OPTIONS] SRC_PATH|- CONTAINER:DEST_PATH`

:warning: `nerdctl cp` is designed only for use with trusted, cooperating containers.
Using `nerdctl cp` with untrusted or malicious containers is unsupported and may not provide protection against unexpected behavior.

Flags:
- :whale: `-L, --follow-link` Always follow symbol link in SRC_PATH.

Unimplemented `docker cp` flags: `--archive`

### :whale: :blue_square: nerdctl ps
List containers.

Usage: `nerdctl ps [OPTIONS]`

Flags:
- :whale: `-a, --all`: Show all containers (default shows just running)
- :whale: `--no-trunc`: Don't truncate output
- :whale: `-q, --quiet`: Only display container IDs
- :whale: `-s, --size`: Display total file sizes
- :whale: `--format`: Format the output using the given Go template
  - :whale: `--format=table` (default): Table
  - :whale: `--format='{{json .}}'`: JSON
  - :nerd_face: `--format=wide`: Wide table
  - :nerd_face: `--format=json`: Alias of `--format='{{json .}}'`
- :whale: `-n, --last`: Show n last created containers (includes all states)
- :whale: `-l, --latest`: Show the latest created container (includes all states)
- :whale: `-f, --filter`: Filter containers based on given conditions
  - :whale: `--filter id=<value>`: Container's ID. Both full ID and
    truncated ID are supported
  - :whale: `--filter name=<value>`: Container's name
  - :whale: `--filter label=<key>=<value>`: Arbitrary string either a key or a
    key-value pair
  - :whale: `--filter exited=<value>`: Container's exit code. Only work with
    `--all`
  - :whale: `--filter status=<value>`: One of `created, running, paused,
    stopped, exited, pausing, unknown`. Note that `restarting, removing, dead` are
    not supported and will be ignored
  - :whale: `--filter before/since=<ID/name>`: Filter containers created before
    or after a given ID or name
  - :whale: `--filter volume=<value>`: Filter by a given mounted volume or bind
    mount
  - :whale: `--filter network=<value>`: Filter by a given network

Following arguments for `--filter` are not supported yet:

1. `--filter ancestor=<value>`
2. `--filter publish/expose=<port/startport-endport>[/<proto>]`
3. `--filter health=<value>`
4. `--filter isolation=<value>`
5. `--filter is-task=<value>`

### :whale: :blue_square: nerdctl inspect
Display detailed information on one or more containers.

Usage: `nerdctl inspect [OPTIONS] NAME|ID [NAME|ID...]`

Flags:
- :nerd_face: `--mode=(dockercompat|native)`: Inspection mode. "native" produces more information.
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`
- :whale: `--type`: Return JSON for specified type

Unimplemented `docker inspect` flags:  `--size`

### :whale: nerdctl logs
Fetch the logs of a container.

:warning: Currently, only containers created with `nerdctl run -d` are supported.

Usage: `nerdctl logs [OPTIONS] CONTAINER`

Flags:
- :whale: `--f, --follow`: Follow log output
- :whale: `--since`: Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)
- :whale: `--until`: Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)
- :whale: `-t, --timestamps`: Show timestamps
- :whale: `-n, --tail`: Number of lines to show from the end of the logs (default "all")

Unimplemented `docker logs` flags: `--details`

### :whale: nerdctl port
List port mappings or a specific mapping for the container.

Usage: `nerdctl port CONTAINER [PRIVATE_PORT[/PROTO]]`

### :whale: nerdctl rm
Remove one or more containers.

Usage: `nerdctl rm [OPTIONS] CONTAINER [CONTAINER...]`

Flags:
- :whale: `-f, --force`: Force the removal of a running|paused|unknown container (uses SIGKILL)
- :whale: `-v, --volumes`: Remove anonymous volumes associated with the container

Unimplemented `docker rm` flags: `--link`

### :whale: nerdctl stop
Stop one or more running containers.

Usage: `nerdctl stop [OPTIONS] CONTAINER [CONTAINER...]`

Flags:
- :whale: `-t, --time=SECONDS`: Seconds to wait for stop before killing it (default "10")

### :whale: nerdctl start
Start one or more running containers.

Usage: `nerdctl start [OPTIONS] CONTAINER [CONTAINER...]`

Flags:
- :whale: `-a, --attach`: Attach STDOUT/STDERR and forward signals

Unimplemented `docker start` flags: `--checkpoint`, `--checkpoint-dir`, `--detach-keys`, `--interactive`

### :whale: nerdctl restart
Restart one or more running containers.

Usage: `nerdctl restart [OPTIONS] CONTAINER [CONTAINER...]`

Flags:
- :whale: `-t, --time=SECONDS`: Seconds to wait for stop before killing it (default "10")

### :whale: nerdctl update
Update configuration of one or more containers.

Usage: `nerdctl update [OPTIONS] CONTAINER [CONTAINER...]`

- :whale: `--cpus`: Number of CPUs
- :whale: `--cpu-quota`: Limit the CPU CFS (Completely Fair Scheduler) quota
- :whale: `--cpu-period`: Limit the CPU CFS (Completely Fair Scheduler) period
- :whale: `--cpu-shares`: CPU shares (relative weight)
- :whale: `--cpuset-cpus`: CPUs in which to allow execution (0-3, 0,1)
- :whale: `--cpuset-mems`: Memory nodes (MEMs) in which to allow execution (0-3, 0,1). Only effective on NUMA systems
- :whale: `--memory`: Memory limit
- :whale: `--memory-reservation`: Memory soft limit
- :whale: `--memory-swap`: Swap limit equal to memory plus swap: '-1' to enable unlimited swap
- :whale: `--kernel-memory`: Kernel memory limit (deprecated)
- :whale: `--pids-limit`: Tune container pids limit
- :whale: `--blkio-weight`: Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)

### :whale: nerdctl wait
Block until one or more containers stop, then print their exit codes.

Usage: `nerdctl wait CONTAINER [CONTAINER...]`

### :whale: nerdctl kill
Kill one or more running containers.

Usage: `nerdctl kill [OPTIONS] CONTAINER [CONTAINER...]`

Flags:
- :whale: `-s, --signal`: Signal to send to the container (default: "KILL")

### :whale: nerdctl pause
Pause all processes within one or more containers.

Usage: `nerdctl pause CONTAINER [CONTAINER...]`

### :whale: nerdctl unpause
Unpause all processes within one or more containers.

Usage: `nerdctl unpause CONTAINER [CONTAINER...]`

### :whale: docker rename
Rename a container.

Usage: `nerdctl rename CONTAINER NEW_NAME`

### :whale: nerdctl container prune
Remove all stopped containers.

Usage: `nerdctl container prune [OPTIONS]`

Flags:
- :whale: `-f, --force`: Do not prompt for confirmation.

Unimplemented `docker container prune` flags: `--filter`

## Build
### :whale: nerdctl build
Build an image from a Dockerfile.

:information_source: Needs buildkitd to be running. See also [the document about setting up `nerdctl build` with BuildKit](./docs/build.md).

Usage: `nerdctl build [OPTIONS] PATH`

Flags:
- :nerd_face: `--buildkit-host=<BUILDKIT_HOST>`: BuildKit address
- :whale: `-t, --tag`: Name and optionally a tag in the 'name:tag' format
- :whale: `-f, --file`: Name of the Dockerfile
- :whale: `--target`: Set the target build stage to build
- :whale: `--build-arg`: Set build-time variables
- :whale: `--no-cache`: Do not use cache when building the image
- :whale: `--output=OUTPUT`: Output destination (format: type=local,dest=path)
  - :whale: `type=local,dest=path/to/output-dir`: Local directory
  - :whale: `type=oci[,dest=path/to/output.tar]`: Docker/OCI dual-format tar ball (compatible with `docker buildx build`)
  - :whale: `type=docker[,dest=path/to/output.tar]`: Docker format tar ball (compatible with `docker buildx build`)
  - :whale: `type=tar[,dest=path/to/output.tar]`: Raw tar ball
  - :whale: `type=image,name=example.com/image,push=true`: Push to a registry (see [`buildctl build`](https://github.com/moby/buildkit/tree/v0.9.0#imageregistry) documentation)
- :whale: `--progress=(auto|plain|tty)`: Set type of progress output (auto, plain, tty). Use plain to show container output
- :whale: `--secret`: Secret file to expose to the build: id=mysecret,src=/local/secret
- :whale: `--ssh`: SSH agent socket or keys to expose to the build (format: `default|<id>[=<socket>|<key>[,<key>]]`)
- :whale: `-q, --quiet`: Suppress the build output and print image ID on success
- :whale: `--cache-from=CACHE`: External cache sources (eg. user/app:cache, type=local,src=path/to/dir) (compatible with `docker buildx build`)
- :whale: `--cache-to=CACHE`: Cache export destinations (eg. user/app:cache, type=local,dest=path/to/dir) (compatible with `docker buildx build`)
- :whale: `--platform=(amd64|arm64|...)`: Set target platform for build (compatible with `docker buildx build`)
- :whale: `--iidfile=FILE`: Write the image ID to the file
- :nerd_face: `--ipfs`: Build image with pulling base images from IPFS. See [`./docs/ipfs.md`](./docs/ipfs.md) for details.
- :whale: `--label`: Set metadata for an image

Unimplemented `docker build` flags: `--add-host`, `--network`, `--squash`

### :whale: nerdctl commit
Create a new image from a container's changes

Usage: `nerdctl commit [OPTIONS] CONTAINER [REPOSITORY[:TAG]]`

Flags:
- :whale: `-a, --author`: Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")
- :whale: `-m, --message`: Commit message
- :whale: `-c, --change`: Apply Dockerfile instruction to the created image (supported directives: [CMD, ENTRYPOINT])
- :whale: `-p, --pause`: Pause container during commit (default: true)

## Image management

### :whale: :blue_square: nerdctl images
List images

:warning: The image ID is usually different from Docker image ID.

Usage: `nerdctl images [OPTIONS] [REPOSITORY[:TAG]]`

Flags:
- :whale: `-a, --all`: Show all images (unimplemented)
- :whale: `-q, --quiet`: Only show numeric IDs
- :whale: `--no-trunc`: Don't truncate output
- :whale: `--format`: Format the output using the given Go template
  - :whale: `--format=table` (default): Table
  - :whale: `--format='{{json .}}'`: JSON
  - :nerd_face: `--format=wide`: Wide table
  - :nerd_face: `--format=json`: Alias of `--format='{{json .}}'`
- :whale: `--digests`: Show digests (compatible with Docker, unlike ID)
- :whale: `-f, --filter`: Filter the images. For now, only 'before=<image:tag>' and 'since=<image:tag>' is supported.
  - :whale: `--filter=before=<image:tag>`: Images created before given image (exclusive)
  - :whale: `--filter=since=<image:tag>`: Images created after given image (exclusive)
- :nerd_face: `--names`: Show image names

Following arguments for `--filter` are not supported yet:

1. `--filter=label=<key>=<value>`: Filter images by label
2. `--filter=reference=<image:tag>`: Filter images by reference
3. `--filter=dangling=true`: Filter images by dangling

### :whale: :blue_square: nerdctl pull
Pull an image from a registry.

Usage: `nerdctl pull [OPTIONS] NAME[:TAG|@DIGEST]`

:nerd_face: `ipfs://` prefix can be used for `NAME` to pull it from IPFS. See [`/docs/ipfs.md`](./docs/ipfs.md) for details.

Flags:
- :whale: `--platform=(amd64|arm64|...)`: Pull content for a specific platform
  - :nerd_face: Unlike Docker, this flag can be specified multiple times (`--platform=amd64 --platform=arm64`)
- :nerd_face: `--all-platforms`: Pull content for all platforms
- :nerd_face: `--unpack`: Unpack the image for the current single platform (auto/true/false)
- :whale: `-q, --quiet`: Suppress verbose output
- :nerd_face: `--verify`: Verify the image (none|cosign). See [`docs/cosign.md`](./docs/cosign.md) for details.
- :nerd_face: `--cosign-key`: Path to the public key file, KMS, URI or Kubernetes Secret for `--verify=cosign`

Unimplemented `docker pull` flags: `--all-tags`, `--disable-content-trust` (default true)

### :whale: nerdctl push
Push an image to a registry.

Usage: `nerdctl push [OPTIONS] NAME[:TAG]`

:nerd_face: `ipfs://` prefix can be used for `NAME` to push it to IPFS. See [`/docs/ipfs.md`](./docs/ipfs.md) for details.

Flags:
- :nerd_face: `--platform=(amd64|arm64|...)`: Push content for a specific platform
- :nerd_face: `--all-platforms`: Push content for all platforms
- :nerd_face: `--sign`: Sign the image (none|cosign). See [`docs/cosign.md`](./docs/cosign.md) for details.
- :nerd_face: `--cosign-key`: Path to the private key file, KMS, URI or Kubernetes Secret for `--sign=cosign`
- :nerd_face: `--allow-nondistributable-artifacts`: Allow pushing images with non-distributable blobs
- :nerd_face: `--ipfs-address`: Multiaddr of IPFS API (default uses `$IPFS_PATH` env variable if defined or local directory `~/.ipfs`)

Unimplemented `docker push` flags: `--all-tags`, `--disable-content-trust` (default true), `--quiet`

### :whale: nerdctl load
Load an image from a tar archive or STDIN.

:nerd_face: Supports both Docker Image Spec v1.2 and OCI Image Spec v1.0.

Usage: `nerdctl load [OPTIONS]`

Flags:
- :whale: `-i, --input`: Read from tar archive file, instead of STDIN
- :nerd_face: `--platform=(amd64|arm64|...)`: Import content for a specific platform
- :nerd_face: `--all-platforms`: Import content for all platforms

Unimplemented `docker load` flags: `--quiet`

### :whale: nerdctl save
Save one or more images to a tar archive (streamed to STDOUT by default)

:nerd_face: The archive implements both Docker Image Spec v1.2 and OCI Image Spec v1.0.

Usage: `nerdctl save [OPTIONS] IMAGE [IMAGE...]`

Flags:
- :whale: `-o, --output`: Write to a file, instead of STDOUT
- :nerd_face: `--platform=(amd64|arm64|...)`: Export content for a specific platform
- :nerd_face: `--all-platforms`: Export content for all platforms

### :whale: nerdctl tag
Create a tag TARGET\_IMAGE that refers to SOURCE\_IMAGE.

Usage: `nerdctl tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

### :whale: nerdctl rmi
Remove one or more images

Usage: `nerdctl rmi [OPTIONS] IMAGE [IMAGE...]`

Flags:
- :nerd_face: `--async`: Asynchronous mode
- :whale: `-f, --force`: Force removal of the image

Unimplemented `docker rmi` flags: `--no-prune`

### :whale: nerdctl image inspect
Display detailed information on one or more images.

Usage: `nerdctl image inspect [OPTIONS] NAME|ID [NAME|ID...]`

Flags:
- :nerd_face: `--mode=(dockercompat|native)`: Inspection mode. "native" produces more information.
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`
- :nerd_face: `--platform=(amd64|arm64|...)`: Inspect a specific platform

### :whale: nerdctl image history
Show the history of an image.

Usage: `nerdctl history [OPTIONS] IMAGE`

Flags:
- :whale: `--no-trunc`: Don't truncate output
- :whale: `-q, --quiet`: Only display snapshots IDs
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

### :whale: nerdctl image prune
Remove unused images.

:warning: Currently, `nerdctl image prune` requires --all to be specified.

Usage: `nerdctl image prune [OPTIONS]`

Flags:
- :whale: `-a, --all`: Remove all unused images, not just dangling ones
- :whale: `-f, --force`: Do not prompt for confirmation

Unimplemented `docker image prune` flags: `--filter`

### :nerd_face: nerdctl image convert
Convert an image format.

e.g., `nerdctl image convert --estargz --oci example.com/foo:orig example.com/foo:esgz`

Usage: `nerdctl image convert [OPTIONS] SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

Flags:
-  `--estargz`                          : convert legacy tar(.gz) layers to eStargz for lazy pulling. Should be used in conjunction with '--oci'
-  `--estargz-record-in=<FILE>`         : read `ctr-remote optimize --record-out=<FILE>` record file. :warning: This flag is experimental and subject to change.
-  `--estargz-compression-level=<LEVEL>`: eStargz compression level (default: 9)
-  `--estargz-chunk-size=<SIZE>`        : eStargz chunk size
-  `--zstdchunked`                      : Use zstd compression instead of gzip (a.k.a zstd:chunked). Should be used in conjunction with '--oci'
-  `--uncompress`                       : convert tar.gz layers to uncompressed tar layers
-  `--oci`                              : convert Docker media types to OCI media types
-  `--platform=<PLATFORM>`              : convert content for a specific platform
-  `--all-platforms`                    : convert content for all platforms (default: false)

### :nerd_face: nerdctl image encrypt
Encrypt image layers. See [`./docs/ocicrypt.md`](./docs/ocicrypt.md).

Usage: `nerdctl image encrypt [OPTIONS] SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

Example:
```bash
openssl genrsa -out mykey.pem
openssl rsa -in mykey.pem -pubout -out mypubkey.pem
nerdctl image encrypt --recipient=jwe:mypubkey.pem --platform=linux/amd64,linux/arm64 foo example.com/foo:encrypted
nerdctl push example.com/foo:encrypted
```

:warning: CAUTION: This command only encrypts image layers, but does NOT encrypt [container configuration such as `Env` and `Cmd`](https://github.com/opencontainers/image-spec/blob/v1.0.1/config.md#example).
To see non-encrypted information, run `nerdctl image inspect --mode=native --platform=PLATFORM example.com/foo:encrypted` .

Flags:
-  `--recipient=<RECIPIENT>`      : Recipient of the image is the person who can decrypt (e.g., `jwe:mypubkey.pem`)
-  `--dec-recipient=<RECIPIENT>`  : Recipient of the image; used only for PKCS7 and must be an x509 certificate
-  `--key=<KEY>[:<PWDDESC>]`      : A secret key's filename and an optional password separated by colon, PWDDESC=<password>|pass:<password>|fd=<file descriptor>|filename
-  `--gpg-homedir=<DIR>`          : The GPG homedir to use; by default gpg uses ~/.gnupg
-  `--gpg-version=<VERSION>`      : The GPG version ("v1" or "v2"), default will make an educated guess
-  `--platform=<PLATFORM>`        : Convert content for a specific platform
-  `--all-platforms`              : Convert content for all platforms (default: false)


### :nerd_face: nerdctl image decrypt
Decrypt image layers. See [`./docs/ocicrypt.md`](./docs/ocicrypt.md).

Usage: `nerdctl image decrypt [OPTIONS] SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

Example:
```bash
nerdctl pull --unpack=false example.com/foo:encrypted
nerdctl image decrypt --key=mykey.pem example.com/foo:encrypted foo:decrypted
```

Flags:
-  `--dec-recipient=<RECIPIENT>`  : Recipient of the image; used only for PKCS7 and must be an x509 certificate
-  `--key=<KEY>[:<PWDDESC>]`      : A secret key's filename and an optional password separated by colon, PWDDESC=<password>|pass:<password>|fd=<file descriptor>|filename
-  `--gpg-homedir=<DIR>`          : The GPG homedir to use; by default gpg uses ~/.gnupg
-  `--gpg-version=<VERSION>`      : The GPG version ("v1" or "v2"), default will make an educated guess
-  `--platform=<PLATFORM>`        : Convert content for a specific platform
-  `--all-platforms`              : Convert content for all platforms (default: false)

## Registry
### :whale: nerdctl login
Log in to a container registry.

Usage: `nerdctl login [OPTIONS] [SERVER]`

Flags:
- :whale: `-u, --username`:   Username
- :whale: `-p, --password`:   Password
- :whale: `--password-stdin`: Take the password from stdin

### :whale: nerdctl logout
Log out from a container registry

Usage: `nerdctl logout [SERVER]`

## Network management
### :whale: nerdctl network create
Create a network

:information_source: To isolate CNI bridge, CNI plugins v1.1.0 or later needs to be installed.

Usage: `nerdctl network create [OPTIONS] NETWORK`

Flags:
- :whale: `-d, --driver=(bridge|nat|macvlan|ipvlan)`: Driver to manage the Network
    - :whale: `--driver=bridge`: Default driver for unix
    - :whale: `--driver=macvlan`: Macvlan network driver for unix
    - :whale: `--driver=ipvlan`: IPvlan network driver for unix
    - :whale: :blue_square: `--driver=nat`: Default driver for windows
- :whale: `-o, --opt`: Set driver specific options
    - :whale: `--opt=com.docker.network.driver.mtu=<MTU>`: Set the containers network MTU
    - :nerd_face: `--opt=mtu=<MTU>`: Alias of `--opt=com.docker.network.driver.mtu=<MTU>`
    - :whale: `--opt=macvlan_mode=(bridge)>`: Set macvlan network mode (default: bridge)
    - :whale: `--opt=ipvlan_mode=(l2|l3)`: Set IPvlan network mode (default: l2)
    - :nerd_face: `--opt=mode=(bridge|l2|l3)`: Alias of `--opt=macvlan_mode=(bridge)` and `--opt=ipvlan_mode=(l2|l3)`
    - :whale: `--opt=parent=<INTERFACE>`: Set valid parent interface on host
- :whale: `--ipam-driver=(default|host-local|dhcp)`: IP Address Management Driver
    - :whale: :blue_square: `--ipam-driver=default`: Default IPAM driver
    - :nerd_face: `--ipam-driver=host-local`: Host-local IPAM driver for unix
    - :nerd_face: `--ipam-driver=dhcp`: DHCP IPAM driver for unix, requires root
- :whale: `--ipam-opt`: Set IPAM driver specific options
- :whale: `--subnet`: Subnet in CIDR format that represents a network segment, e.g. "10.5.0.0/16"
- :whale: `--gateway`: Gateway for the master subnet
- :whale: `--ip-range`: Allocate container ip from a sub-range
- :whale: `--label`: Set metadata on a network

Unimplemented `docker network create` flags: `--attachable`, `--aux-address`, `--config-from`, `--config-only`, `--ingress`, `--internal`, `--ipv6`, `--scope`

### :whale: nerdctl network ls
List networks

Usage: `nerdctl network ls [OPTIONS]`

Flags:
- :whale: `-q, --quiet`: Only display network IDs
- :whale: `--format`: Format the output using the given Go template
  - :whale: `--format=table` (default): Table
  - :whale: `--format='{{json .}}'`: JSON
  - :nerd_face: `--format=wide`: Alias of `--format=table`
  - :nerd_face: `--format=json`: Alias of `--format='{{json .}}'`

Unimplemented `docker network ls` flags: `--filter`, `--no-trunc`

### :whale: nerdctl network inspect
Display detailed information on one or more networks

Usage: `nerdctl network inspect [OPTIONS] NETWORK [NETWORK...]`

Flags:
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

Unimplemented `docker network inspect` flags: `--verbose`

### :whale: nerdctl network rm
Remove one or more networks

Usage: `nerdctl network rm NETWORK [NETWORK...]`

### :whale: nerdctl network prune
Remove all unused networks

Usage: `nerdctl network prune [OPTIONS]`

Flags:
- :whale: `-f, --force`: Do not prompt for confirmation

Unimplemented `docker network prune` flags: `--filter`

## Volume management
### :whale: nerdctl volume create
Create a volume

Usage: `nerdctl volume create [OPTIONS] [VOLUME]`

Flags:
- :whale: `--label`: Set metadata for a volume

Unimplemented `docker volume create` flags: `--driver`, `--opt`

### :whale: nerdctl volume ls
List volumes

Usage: `nerdctl volume ls [OPTIONS]`

Flags:
- :whale: `-q, --quiet`: Only display volume names
- :whale: `--format`: Format the output using the given Go template
  - :whale: `--format=table` (default): Table
  - :whale: `--format='{{json .}}'`: JSON
  - :nerd_face: `--format=wide`: Alias of `--format=table`
  - :nerd_face: `--format=json`: Alias of `--format='{{json .}}'`
- :nerd_face: `--size`: Display the disk usage of volumes.
- :whale: `-f, --filter`: Filter volumes based on given conditions.
    - :whale: `--filter label=<key>=<value>`: Matches volumes by label on both
      `key` and `value`. If `value` is left empty, matches all volumes with `key`
      regardless of its value
    - :whale: `--filter name=<value>`: Matches all volumes with a name containing
      the `value` string
    - :nerd_face: `--filter "size=<value>"`: Matches all volumes with a size
      meets the `value`. `size` operand can be `>=, <=, >, <, =` and `value` must be
      an integer. Quotes should be used otherwise some shells may treat operand as
      redirections

Following arguments for `--filter` are not supported yet:

1. `--filter=dangling=true`: Filter volumes by dangling
2. `--filter=driver=local`: Filter volumes by driver

### :whale: nerdctl volume inspect
Display detailed information on one or more volumes

Usage: `nerdctl volume inspect [OPTIONS] VOLUME [VOLUME...]`

Flags:
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`
- :nerd_face: `--size`: Displays disk usage of volume

### :whale: nerdctl volume rm
Remove one or more volumes

Usage: `nerdctl volume rm [OPTIONS] VOLUME [VOLUME...]`

- :whale: `-f, --force`: Force the removal of one or more volumes

### :whale: nerdctl volume prune
Remove all unused local volumes

Usage: `nerdctl volume prune [OPTIONS]`

Flags:
- :whale: `-f, --force`: Do not prompt for confirmation

Unimplemented `docker volume prune` flags: `--filter`

## Namespace management

### :nerd_face: :blue_square: nerdctl namespace create
Create a new namespace.

Usage: `nerdctl namespace create NAMESPACE`
Flags:
- `--label`: Set labels for a namespace

### :nerd_face: :blue_square: nerdctl namespace inspect
Inspect a namespace.

Usage: `nerdctl namespace inspect NAMESPACE`

### :nerd_face: :blue_square: nerdctl namespace ls
List containerd namespaces such as "default", "moby", or "k8s.io".

Usage: `nerdctl namespace ls [OPTIONS]`

Flags:
- `-q, --quiet`: Only display namespace names

### :nerd_face: :blue_square: nerdctl namespace remove
Remove one or more namespaces.

Usage: `nerdctl namespace remove [OPTIONS] NAMESPACE [NAMESPACE...]`

Flags:
- `-c, --cgroup`: delete the namespace's cgroup

### :nerd_face: :blue_square: nerdctl namespace update
Udapte labels for a namespace.

Usage: `nerdctl namespace update NAMESPACE`

Flags:
- `--label`: Set labels for a namespace

## AppArmor profile management
### :nerd_face: nerdctl apparmor inspect
Display the default AppArmor profile "nerdctl-default". Other profiles cannot be displayed with this command.

Usage: `nerdctl apparmor inspect`

### :nerd_face: nerdctl apparmor load
Load the default AppArmor profile "nerdctl-default". Requires root.

Usage: `nerdctl apparmor load`

### :nerd_face: nerdctl apparmor ls
List the loaded AppArmor profile

Usage: `nerdctl apparmor ls [OPTIONS]`

Flags:
- `-q, --quiet`: Only display volume names
- `--format`: Format the output using the given Go template, e.g, `{{json .}}`

### :nerd_face: nerdctl apparmor unload
Unload an AppArmor profile. The target profile name defaults to "nerdctl-default". Requires root.

Usage: `nerdctl apparmor unload [PROFILE]`

## Builder management
### :whale: nerdctl builder prune
Clean up BuildKit build cache.

:warning: The output format is not compatible with Docker.

Usage: `nerdctl builder prune`

Flags:
- :nerd_face: `--buildkit-host=<BUILDKIT_HOST>`: BuildKit address

### :nerd_face: nerdctl builder debug
Interactive debugging of Dockerfile using [buildg](https://github.com/ktock/buildg).
Please refer to [`./docs/builder-debug.md`](./docs/builder-debug.md) for details.
This is an [experimental](./docs/experimental.md) feature.

:warning: This command currently doesn't use the host's `buildkitd` daemon but uses the patched version of BuildKit provided by buildg. This should be fixed in the future.

Usage: `nerdctl builder debug PATH`

Flags:
- :nerd_face: `-f`, `--file`: Name of the Dockerfile
- :nerd_face: `--image`: Image to use for debugging stage
- :nerd_face: `--target`: Set the target build stage to build
- :nerd_face: `--build-arg`: Set build-time variables

Unimplemented `docker builder prune` flags: `--all`, `--filter`, `--force`, `--keep-storage`

## System
### :whale: nerdctl events
Get real time events from the server.

:warning: The output format is not compatible with Docker.

Usage: `nerdctl events [OPTIONS]`

Flags:
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

Unimplemented `docker events` flags: `--filter`, `--since`, `--until`

### :whale: nerdctl info
Display system-wide information

Usage: `nerdctl info [OPTIONS]`

Flags:
- :whale: `-f, --format`: Format the output using the given Go template, e.g, `{{json .}}`
- :nerd_face: `--mode=(dockercompat|native)`: Information mode. "native" produces more information.

### :whale: nerdctl version
Show the nerdctl version information

Usage: `nerdctl version [OPTIONS]`

Flags:
- :whale: `-f, --format`: Format the output using the given Go template, e.g, `{{json .}}`

### :whale: nerdctl system prune
Remove unused data

:warning: Currently, `nerdctl system prune` requires --all to be specified.

Usage: `nerdctl system prune [OPTIONS]`

Flags:
- :whale: `-a, --all`: Remove all unused images, not just dangling ones
- :whale: `-f, --force`: Do not prompt for confirmation
- :whale: `--volumes`: Prune volumes

Unimplemented `docker system prune` flags: `--filter`

## Stats
### :whale: nerdctl stats
Display a live stream of container(s) resource usage statistics.

Usage: `nerdctl stats [OPTIONS]`

Flags:
- :whale: `-a, --all`: Show all containers (default shows just running)
- :whale: `--format=FORMAT`: Pretty-print images using a Go template, e.g., `{{json .}}`
- :whale: `--no-stream`: Disable streaming stats and only pull the first result
- :whale: `--no-trunc `: Do not truncate output

### :whale: nerdctl top
Display the running processes of a container.


Usage: `nerdctl top CONTAINER [ps OPTIONS]`


## Shell completion

### :nerd_face: nerdctl completion bash
Generate the autocompletion script for bash.

Usage: add the following line to `~/.bash_profile`:
```bash
source <(nerdctl completion bash)
```

Or run `nerdctl completion bash > /etc/bash_completion.d/nerdctl` as the root.

### :nerd_face: nerdctl completion zsh
Generate the autocompletion script for zsh.

Usage: see `nerdctl completion zsh --help`

### :nerd_face: nerdctl completion fish
Generate the autocompletion script for fish.

Usage: see `nerdctl completion fish --help`

### :nerd_face: nerdctl completion powershell
Generate the autocompletion script for powershell.

Usage: see `nerdctl completion powershell --help`

## Compose

### :whale: nerdctl compose
Compose

Usage: `nerdctl compose [OPTIONS] [COMMAND]`

Flags:
- :whale: `-f, --file`: Specify an alternate compose file
- :whale: `-p, --project-name`: Specify an alternate project name

### :whale: nerdctl compose up
Create and start containers

Usage: `nerdctl compose up [OPTIONS] [SERVICE...]`

Flags:
- :whale: `-d, --detach`: Detached mode: Run containers in the background
- :whale: `--no-build`: Don't build an image, even if it's missing.
- :whale: `--no-color`: Produce monochrome output
- :whale: `--no-log-prefix`: Don't print prefix in logs
- :whale: `--build`: Build images before starting containers.
- :nerd_face: `--ipfs`: Build images with pulling base images from IPFS. See [`./docs/ipfs.md`](./docs/ipfs.md) for details.
- :whale: `--quiet-pull`: Pull without printing progress information
- :whale: `--scale`: Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.
- :whale: `--remove-orphans`: Remove containers for services not defined in the Compose file

Unimplemented `docker-compose up` (V1) flags: `--no-deps`, `--force-recreate`, `--always-recreate-deps`, `--no-recreate`,
`--no-start`, `--abort-on-container-exit`, `--attach-dependencies`, `--timeout`, `--renew-anon-volumes`, `--exit-code-from`

Unimplemented `docker compose up` (V2) flags: `--environment`

### :whale: nerdctl compose logs
Create and start containers

Usage: `nerdctl compose logs [OPTIONS]`

Flags:
- :whale: `--no-color`: Produce monochrome output
- :whale: `--no-log-prefix`: Don't print prefix in logs
- :whale: `--timestamps`: Show timestamps
- :whale: `--tail`: Number of lines to show from the end of the logs

Unimplemented `docker compose logs` (V2) flags:  `--since`, `--until`

### :whale: nerdctl compose build
Build or rebuild services.

Usage: `nerdctl compose build [OPTIONS]`

Flags:
- :whale: `--build-arg`: Set build-time variables for services
- :whale: `--no-cache`: Do not use cache when building the image
- :whale: `--progress`: Set type of progress output (auto, plain, tty)
- :nerd_face: `--ipfs`: Build images with pulling base images from IPFS. See [`./docs/ipfs.md`](./docs/ipfs.md) for details.

Unimplemented `docker-compose build` (V1) flags:  `--compress`, `--force-rm`, `--memory`, `--no-rm`, `--parallel`, `--pull`, `--quiet`

### :whale: nerdctl compose down
Remove containers and associated resources

Usage: `nerdctl compose down [OPTIONS]`

Flags:
- :whale: `-v, --volumes`: Remove named volumes declared in the volumes section of the Compose file and anonymous volumes attached to containers

Unimplemented `docker-compose down` (V1) flags: `--rmi`, `--remove-orphans`, `--timeout`

### :whale: nerdctl compose ps
List containers of services

Usage: `nerdctl compose ps`

Unimplemented `docker-compose ps` (V1) flags: `--quiet`, `--services`, `--filter`, `--all`

Unimplemented `docker compose ps` (V2) flags: `--format`, `--status`

### :whale: nerdctl compose pull
Pull service images

Usage: `nerdctl compose pull`

Flags:
- :whale: `-q, --quiet`: Pull without printing progress information

Unimplemented `docker-compose pull` (V1) flags: `--ignore-pull-failures`, `--parallel`, `--no-parallel`, `include-deps`

### :whale: nerdctl compose push
Push service images

Usage: `nerdctl compose push`

Unimplemented `docker-compose pull` (V1) flags: `--ignore-push-failures`

### :whale: nerdctl compose config
Validate and view the Compose file

Usage: `nerdctl compose config`

Flags:
- :whale: `-q, --quiet`: Pull without printing progress information
- :whale: `--services`: Print the service names, one per line.
- :whale: `--volumes`: Print the volume names, one per line.
- :whale: `--hash="*"`: Print the service config hash, one per line.

Unimplemented `docker-compose config` (V1) flags: `--resolve-image-digests`, `--no-interpolate`

Unimplemented `docker compose config` (V2) flags: `--resolve-image-digests`, `--no-interpolate`, `--format`, `--output`, `--profiles`

### :whale: nerdctl compose kill
Force stop service containers

Usage: `nerdctl compose kill`

Flags:
- :whale: `-s, --signal`: SIGNAL to send to the container (default: "SIGKILL")

### :whale: nerdctl compose run
Run a one-off command on a service

Usage: `nerdctl compose run`

Unimplemented `docker-compose run` (V1) flags: `--use-aliases`, `--no-TTY`

Unimplemented `docker compose run` (V2) flags: `--use-aliases`, `--no-TTY`, `--tty`

### :whale: nerdctl compose version
Show the Compose version information (which is the nerdctl version)

Usage: `nerdctl compose version`

Flags:
- :whale: `-f, --format`: Format the output. Values: [pretty | json] (default "pretty")
- :whale: `--short`: Shows only Compose's version number

## IPFS management

P2P image distribution (IPFS) is completely optional. Your host is NOT connected to any P2P network, unless you opt in to [install and run IPFS daemon](https://docs.ipfs.io/install/).

### :nerd_face: nerdctl ipfs registry up
Start read-only local registry backed by IPFS.
See [`./docs/ipfs.md`](./docs/ipfs.md) for details.

Usage: `nerdctl ipfs registry up [OPTIONS]`

Flags:
- :nerd_face: `--listen-registry`: Address to listen (default `localhost:5050`)
- :nerd_face: `--read-retry-num`: Times to retry query on IPFS (default 0 (no retry))
- :nerd_face: `--read-timeout`: Timeout duration of a read request to IPFS (default 0 (no timeout))

### :nerd_face: nerdctl ipfs registry down
Stop and remove read-only local registry backed by IPFS.
See [`./docs/ipfs.md`](./docs/ipfs.md) for details.

Usage: `nerdctl ipfs registry down`

### :nerd_face: nerdctl ipfs registry serve
Serve read-only registry backed by IPFS on localhost.
Use `nerdctl ipfs registry up`.

Usage: `nerdctl ipfs registry serve [OPTIONS]`

Flags:
- :nerd_face: `--ipfs-address`: Multiaddr of IPFS API (default is pulled from `$IPFS_PATH/api` file. If `$IPFS_PATH` env var is not present, it defaults to `~/.ipfs`).
- :nerd_face: `--listen-registry`: Address to listen (default `localhost:5050`).
- :nerd_face: `--read-retry-num`: Times to retry query on IPFS (default 0 (no retry))
- :nerd_face: `--read-timeout`: Timeout duration of a read request to IPFS (default 0 (no timeout))

## Global flags
- :nerd_face: :blue_square: `--address`:  containerd address, optionally with "unix://" prefix
- :nerd_face: :blue_square: `-a`, `--host`, `-H`: deprecated aliases of `--address`
- :nerd_face: :blue_square: `--namespace`: containerd namespace
- :nerd_face: :blue_square: `-n`: deprecated alias of `--namespace`
- :nerd_face: :blue_square: `--snapshotter`: containerd snapshotter
- :nerd_face: :blue_square: `--storage-driver`: deprecated alias of `--snapshotter`
- :nerd_face: :blue_square: `--cni-path`: CNI binary path (default: `/opt/cni/bin`) [`$CNI_PATH`]
- :nerd_face: :blue_square: `--cni-netconfpath`: CNI netconf path (default: `/etc/cni/net.d`) [`$NETCONFPATH`]
- :nerd_face: :blue_square: `--data-root`: nerdctl data root, e.g. "/var/lib/nerdctl"
- :nerd_face: `--cgroup-manager=(cgroupfs|systemd|none)`: cgroup manager
  - Default: "systemd" on cgroup v2 (rootful & rootless), "cgroupfs" on v1 rootful, "none" on v1 rootless
- :nerd_face: `--insecure-registry`: skips verifying HTTPS certs, and allows falling back to plain HTTP

The global flags can be also specified in `/etc/nerdctl/nerdctl.toml` (rootful) and `~/.config/nerdctl/nerdctl.toml` (rootless).
See [`./docs/config.md`](./docs/config.md).

## Unimplemented Docker commands
Container management:
- `docker attach`
- `docker diff`
- `docker checkpoint *`

Image:
- `docker export` and `docker import`
- `docker trust *` (Instead, nerdctl supports `nerdctl pull --verify=cosign` and `nerdctl push --sign=cosign`. See [`./docs/cosign.md`](docs/cosign.md).)
- `docker manifest *`

Network management:
- `docker network connect`
- `docker network disconnect`

Registry:
- `docker search`

Compose:
- `docker-compose create|events|exec|images|pause|port|restart|rm|scale|start|stop|top|unpause`

Others:
- `docker system df`
- `docker context`
- Swarm commands are unimplemented and will not be implemented: `docker swarm|node|service|config|secret|stack *`
- Plugin commands are unimplemented and will not be implemented: `docker plugin *`

- - -

# Additional documents
Configuration guide:
- [`./docs/config.md`](./docs/config.md): Configuration (`/etc/nerdctl/nerdctl.toml`, `~/.config/nerdctl/nerdctl.toml`)
- [`./docs/registry.md`](./docs/registry.md): Registry authentication (`~/.docker/config.json`)

Basic features:
- [`./docs/compose.md`](./docs/compose.md):   Compose
- [`./docs/rootless.md`](./docs/rootless.md): Rootless mode
- [`./docs/cni.md`](./docs/cni.md): CNI for containers network
- [`./docs/build.md`](./docs/build.md): `nerdctl build` with BuildKit

Advanced features:
- [`./docs/stargz.md`](./docs/stargz.md):     Lazy-pulling using Stargz Snapshotter
- [`./docs/nydus.md`](./docs/nydus.md):       Lazy-pulling using Nydus Snapshotter
- [`./docs/overlaybd.md`](./docs/overlaybd.md):       Lazy-pulling using OverlayBD Snapshotter
- [`./docs/ocicrypt.md`](./docs/ocicrypt.md): Running encrypted images
- [`./docs/gpu.md`](./docs/gpu.md):           Using GPUs inside containers
- [`./docs/multi-platform.md`](./docs/multi-platform.md):  Multi-platform mode

Experimental features:
- [`./docs/experimental.md`](./docs/experimental.md):  Experimental features
- [`./docs/freebsd.md`](./docs/freebsd.md):  Running FreeBSD jails
- [`./docs/ipfs.md`](./docs/ipfs.md): Distributing images on IPFS
- [`./docs/builder-debug.md`](./docs/builder-debug.md): Interactive debugging of Dockerfile

Implementation details:
- [`./docs/dir.md`](./docs/dir.md):           Directory layout (`/var/lib/nerdctl`)

Misc:
- [`./docs/faq.md`](./docs/faq.md): FAQs and Troubleshooting
