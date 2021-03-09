[[⬇️ **Download]**](https://github.com/AkihiroSuda/nerdctl/releases)
[[📖 **Command reference]**](#command-reference)
[[📚 **Additional documents]**](#additional-documents)

# nerdctl: Docker-compatible CLI for containerd

`nerdctl` is a Docker-compatible CLI for [contai**nerd**](https://containerd.io).

 ✅ Same UI/UX as `docker`

 ✅ Supports [rootless mode](./docs/rootless.md)

 ✅ Supports [lazy-pulling (Stargz)](./docs/stargz.md)

 ✅ Supports [encrypted images (ocicrypt)](./docs/ocicrypt.md)

## Examples

### Basic usage

To run a container with the default CNI network (10.4.0.0/24):
```console
# nerdctl run -it --rm alpine
```

To build an image using BuildKit:
```console
# nerdctl build -t foo .
# nerdctl run -it --rm foo
```

### Debugging Kubernetes

To list Kubernetes containers:
```console
# nerdctl --namespace k8s.io ps -a
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
Binaries are available for amd64, arm64, and arm-v7: https://github.com/AkihiroSuda/nerdctl/releases

In addition to containerd, the following components should be installed (optional):
- [CNI plugins](https://github.com/containernetworking/plugins): for using `nerdctl run`.
- [CNI isolation plugin](https://github.com/AkihiroSuda/cni-isolation): for isolating bridge networks (`nerdctl network create`)
- [BuildKit](https://github.com/moby/buildkit): for using `nerdctl build`. BuildKit daemon (`buildkitd`) needs to be running.
- [RootlessKit](https://github.com/rootless-containers/rootlesskit) and [slirp4netns](https://github.com/rootless-containers/slirp4netns): for [Rootless mode](./docs/rootless.md)
   - RootlessKit needs to be v0.10.0 or later. v0.13.2 or later is recommended.
   - slirp4netns needs toe be v0.4.0 or later. v1.1.7 or later is recommended.

These dependencies are included in `nerdctl-full-<VERSION>-<OS>-<ARCH>.tar.gz`, but not included in `nerdctl-<VERSION>-<OS>-<ARCH>.tar.gz`.

To run nerdctl inside Docker:
```bash
docker build -t nerdctl .
docker run -it --rm --privileged nerdctl
```

## Motivation

The goal of `nerdctl` is to facilitate experimenting the cutting-edge features of containerd that are not present in Docker.

Such features includes, but not limited to, [lazy-pulling](./docs/stargz.md) and [encryption of images](./docs/ocicrypt.md).

Note that competing with Docker is _not_ the goal of `nerdctl`. Those cutting-edge features are expected to be eventually available in Docker as well.

Also, `nerdctl` might be potentially useful for debugging Kubernetes clusters, but it is not the primary goal.

## Features present in `nerdctl` but not present in Docker
Major:
- [Lazy-pulling using Stargz Snapshotter](./docs/stargz.md): `nerdctl --snapshotter=stargz run` .
- [Running encrypted images using ocicrypt (imgcrypt)](./docs/ocicrypt.md)

Minor:
- Namespacing: `nerdctl --namespace=<NS> ps` . 
  (NOTE: All Kubernetes containers are in the `k8s.io` containerd namespace regardless to Kubernetes namespaces)
- Exporting Docker/OCI dual-format archives: `nerdctl save` .
- Importing OCI archives as well as Docker archives: `nerdctl load` .
- Specifying a non-image rootfs: `nerdctl run -it --rootfs <ROOTFS> /bin/sh` . The CLI syntax conforms to Podman convention.

Trivial:
- Inspecting raw OCI config: `nerdctl container inspect --mode=native` .

## Similar tools

- [`ctr`](https://github.com/containerd/containerd/tree/master/cmd/ctr): incompatible with Docker CLI, and not friendly to users.
  Notably, `ctr` lacks the equivalents of the following Docker CLI commands:
  - `docker run -p <PORT>`
  - `docker run --restart=always --net=bridge`
  - `docker pull` with `~/.docker/config.json` and credential helper binaries such as `docker-credential-ecr-login`
  - `docker logs`

- [`crictl`](https://github.com/kubernetes-sigs/cri-tools): incompatible with Docker CLI, not friendly to users, and does not support non-CRI features
- [k3c v0.2 (abandoned)](https://github.com/rancher/k3c/tree/v0.2.1): needs an extra daemon, and does not support non-CRI features
- [Rancher Kim (nee k3c v0.3)](https://github.com/rancher/kim): needs Kubernetes, and only focuses on image management commands such as `kim build` and `kim push`
- [PouchContainer (abandoned?)](https://github.com/alibaba/pouch): needs an extra daemon

## Developer guide

### Compiling nerdctl from source

Run `make && sudo make install`.

Using `go get github.com/AkihiroSuda/nerdctl` is possible, but unrecommended because it does not fill version strings printed in `nerdctl version`

### Test suite
#### Running test suite against nerdctl
Run `go test -exec sudo -v ./...` after `make && sudo make install`.

For testing rootless mode, `-exec sudo` is not needed.

To run tests in a container:
```bash
docker build -t test --target test .
docker run -t --rm --privileged test
```
#### Running test suite against Docker
Run `go test -exec sudo -test.target=docker .` to ensure that the test suite is compatible with Docker.

### Contributing to nerdctl

Lots of commands and flags are currently missing. Pull requests are highly welcome.

Please certify your [Developer Certificate of Origin (DCO)](https://developercertificate.org/), by signing off your commit with `git commit -s` and with your real name.

- - -

# Command reference

:whale:     = Docker compatible

:nerd_face: = nerdctl specific

Unlisted `docker` CLI flags are unimplemented yet in `nerdctl` CLI.
It does not necessarily mean that the corresponding features are missing in containerd.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


  - [Run & Exec](#run--exec)
    - [:whale: nerdctl run](#whale-nerdctl-run)
    - [:whale: nerdctl exec](#whale-nerdctl-exec)
  - [Container management](#container-management)
    - [:whale: nerdctl ps](#whale-nerdctl-ps)
    - [:whale: nerdctl inspect](#whale-nerdctl-inspect)
    - [:whale: nerdctl logs](#whale-nerdctl-logs)
    - [:whale: nerdctl port](#whale-nerdctl-port)
    - [:whale: nerdctl rm](#whale-nerdctl-rm)
    - [:whale: nerdctl stop](#whale-nerdctl-stop)
    - [:whale: nerdctl start](#whale-nerdctl-start)
    - [:whale: nerdctl kill](#whale-nerdctl-kill)
    - [:whale: nerdctl pause](#whale-nerdctl-pause)
    - [:whale: nerdctl unpause](#whale-nerdctl-unpause)
  - [Build](#build)
    - [:whale: nerdctl build](#whale-nerdctl-build)
    - [:whale: nerdctl commit](#whale-nerdctl-commit)
  - [Image management](#image-management)
    - [:whale: nerdctl images](#whale-nerdctl-images)
    - [:whale: nerdctl pull](#whale-nerdctl-pull)
    - [:whale: nerdctl push](#whale-nerdctl-push)
    - [:whale: nerdctl load](#whale-nerdctl-load)
    - [:whale: nerdctl save](#whale-nerdctl-save)
    - [:whale: nerdctl tag](#whale-nerdctl-tag)
    - [:whale: nerdctl rmi](#whale-nerdctl-rmi)
    - [:nerd_face: nerdctl image convert](#nerd_face-nerdctl-image-convert)
  - [Registry](#registry)
    - [:whale: nerdctl login](#whale-nerdctl-login)
    - [:whale: nerdctl logout](#whale-nerdctl-logout)
  - [Network management](#network-management)
    - [:whale: nerdctl network create](#whale-nerdctl-network-create)
    - [:whale: nerdctl network ls](#whale-nerdctl-network-ls)
    - [:whale: nerdctl network inspect](#whale-nerdctl-network-inspect)
    - [:whale: nerdctl network rm](#whale-nerdctl-network-rm)
  - [Volume management](#volume-management)
    - [:whale: nerdctl volume create](#whale-nerdctl-volume-create)
    - [:whale: nerdctl volume ls](#whale-nerdctl-volume-ls)
    - [:whale: nerdctl volume inspect](#whale-nerdctl-volume-inspect)
    - [:whale: nerdctl volume rm](#whale-nerdctl-volume-rm)
  - [System](#system)
    - [:whale: nerdctl events](#whale-nerdctl-events)
    - [:whale: nerdctl info](#whale-nerdctl-info)
    - [:whale: nerdctl version](#whale-nerdctl-version)
  - [Global flags](#global-flags)
  - [Unimplemented Docker commands](#unimplemented-docker-commands)
- [Additional documents](#additional-documents)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->


 
## Run & Exec
### :whale: nerdctl run
Run a command in a new container.

Basic flags:
- :whale: `-i, --interactive`: Keep STDIN open even if not attached"
- :whale: `-t, --tty`: Allocate a pseudo-TTY
  - :warning: WIP: currently `-t` requires `-i`, and conflicts with `-d`
- :whale: `-d, --detach`: Run container in background and print container ID
- :whale: `--restart=(no|always)`: Restart policy to apply when a container exits
  - Default: "no"
  - :warning: No support for `on-failure` and `unless-stopped`
- :whale: `--rm`: Automatically remove the container when it exits
- :whale: `--pull=(always|missing|never)`: Pull image before running
  - Default: "missing"

Network flags:
- :whale: `--network=(bridge|host|none)`: Connect a container to a network
  - Default: "bridge"
- :whale: `-p, --publish`: Publish a container's port(s) to the host
- :whale: `--dns`: Set custom DNS servers
- :whale: `-h, --hostname`: Container host name

Cgroup flags:
- :whale: `--cpus`: Number of CPUs
- :whale: `--memory`: Memory limit
- :whale: `--pids-limit`: Tune container pids limit
- :whale: `--cgroupns=(host|private)`: Cgroup namespace to use
  - Default: "private" on cgroup v2 hosts, "host" on cgroup v1 hosts

User flags:
- :whale: `-u, --user`: Username or UID (format: <name|uid>[:<group|gid>])

Security flags:
- :whale: `--security-opt seccomp=<PROFILE_JSON_FILE>`: specify custom seccomp profile
- :whale: `--security-opt apparmor=<PROFILE>`: specify custom AppArmor profile
- :whale: `--security-opt no-new-privileges`: disallow privilege escalation, e.g., setuid and file capabilities
- :whale: `--cap-add=<CAP>`: Add Linux capabilities
- :whale: `--cap-drop=<CAP>`: Drop Linux capabilities
- :whale: `--privileged`: Give extended privileges to this container

Runtime flags:
- :whale: `--runtime`: Runtime to use for this container, e.g. \"crun\", or \"io.containerd.runsc.v1\".

Volume flags:
- :whale: `-v, --volume`: Bind mount a volume

Rootfs flags:
- :whale: `--read-only`: Mount the container's root filesystem as read only
- :nerd_face: `--rootfs`: The first argument is not an image but the rootfs to the exploded container.
  Corresponds to Podman CLI.

Env flags:
- :whale: `--entrypoint`: Overwrite the default ENTRYPOINT of the image
- :whale: `-w, --workdir`: Working directory inside the container
- :whale: `-e, --env`: Set environment variables

Metadata flags:
- :whale: `--name`: Assign a name to the container
- :whale: `-l, --label`: Set meta data on a container
- :whale: `--label-file`: Read in a line delimited file of labels

### :whale: nerdctl exec
Run a command in a running container.

- :whale: `-i, --interactive`: Keep STDIN open even if not attached
- :whale: `-t, --tty`: Allocate a pseudo-TTY
  - :warning: WIP: currently `-t` requires `-i`, and conflicts with `-d`
- :whale: `-d, --detach`: Detached mode: run command in the background
- :whale: `-w, --workdir`: Working directory inside the container
- :whale: `-e, --env`: Set environment variables
- :whale: `--privileged`: Give extended privileges to the command

## Container management
### :whale: nerdctl ps
List containers.

Flags:
- :whale: `-a, --all`: Show all containers (default shows just running)
- :whale: `--no-trunc`: Don't truncate output
- :whale: `-q, --quiet`: Only display container IDs

### :whale: nerdctl inspect
Display detailed information on one or more containers.

Flags:
- :nerd_face: `--mode=(dockercompat|native)`: Inspection mode. "native" produces more information.

### :whale: nerdctl logs
Fetch the logs of a container. 

:warning: Currently, only containers created with `nerdctl run -d` are supported.

### :whale: nerdctl port
List port mappings or a specific mapping for the container.

### :whale: nerdctl rm
Remove one or more containers.

Flags:
- :whale: `-f`: Force the removal of a running|paused|unknown container (uses SIGKILL)

### :whale: nerdctl stop
Stop one or more running containers.

### :whale: nerdctl start
Start one or more running containers.

### :whale: nerdctl kill
Kill one or more running containers.

### :whale: nerdctl pause
Pause all processes within one or more containers.

### :whale: nerdctl unpause
Unpause all processes within one or more containers.

## Build
### :whale: nerdctl build
Build an image from a Dockerfile.

:information_source: Needs buildkitd to be running.

Flags:
- :nerd_face: `--buildkit-host=<BUILDKIT_HOST>`: BuildKit address
- :whale: `-t, --tag`: Name and optionally a tag in the 'name:tag' format
- :whale: `-f, --file`: Name of the Dockerfile
- :whale: `--target`: Set the target build stage to build
- :whale: `--build-arg`: Set build-time variables
- :whale: `--no-cache`: Do not use cache when building the image
- :whale: `--progress=(auto|plain|tty)`: Set type of progress output (auto, plain, tty). Use plain to show container output
- :whale: `--secret`: Secret file to expose to the build: id=mysecret,src=/local/secret
- :whale: `--ssh`: SSH agent socket or keys to expose to the build (format: `default|<id>[=<socket>|<key>[,<key>]]`)

### :whale: nerdctl commit
Create a new image from a container's changes

Flags:
- :whale: `-a, --author`: Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")
- :whale: `-m, --message`: Commit message

## Image management

### :whale: nerdctl images
List images

Flags:
- :whale: `-q, --quiet`: Only show numeric IDs
- :whale: `--no-trunc`: Don't truncate output

### :whale: nerdctl pull
Pull an image from a registry.

### :whale: nerdctl push
Pull an image from a registry.

### :whale: nerdctl load
Load an image from a tar archive or STDIN.

:nerd_face: Supports both Docker Image Spec v1.2 and OCI Image Spec v1.0.

Flags:
- :whale: `-i, --input`: Read from tar archive file, instead of STDIN

### :whale: nerdctl save
Save one or more images to a tar archive (streamed to STDOUT by default)

:nerd_face: The archive implements both Docker Image Spec v1.2 and OCI Image Spec v1.0.

Flags:
- :whale: `-o, --output`: Write to a file, instead of STDOUT

### :whale: nerdctl tag
Create a tag TARGET\_IMAGE that refers to SOURCE\_IMAGE.

### :whale: nerdctl rmi
Remove one or more images

### :nerd_face: nerdctl image convert
Convert an image format.

e.g., `nerdctl image convert --estargz --oci example.com/foo:orig example.com/foo:esgz`

Flags:
-  `--estargz`                          : convert legacy tar(.gz) layers to eStargz for lazy pulling. Should be used in conjunction with '--oci'
-  `--estargz-record-in=<FILE>`         : read `ctr-remote optimize --record-out=<FILE>` record file. :warning: This flag is experimental and subject to change.
-  `--estargz-compression-level=<LEVEL>`: eStargz compression level (default: 9)
-  `--estargz-chunk-size=<SIZE>`        : eStargz chunk size
-  `--uncompress`                       : convert tar.gz layers to uncompressed tar layers
-  `--oci`                              : convert Docker media types to OCI media types
-  `--platform=<PLATFORM>`              : convert content for a specific platform
-  `--all-platforms`                    : convert content for all platforms (default: false)

## Registry
### :whale: nerdctl login
Log in to a Docker registry.

Flags:
- :whale: `-u, --username`:   Username
- :whale: `-p, --password`:   Password
- :whale: `--password-stdin`: Take the password from stdin

### :whale: nerdctl logout
Log out from a Docker registry

## Network management
### :whale: nerdctl network create
Create a network

:information_source: To isolate CNI bridge, [CNI isolation plugin](https://github.com/AkihiroSuda/cni-isolation) needs to be installed.

Flags:
- :whale: `--subnet`: Subnet in CIDR format that represents a network segment, e.g. "10.5.0.0/16" 

### :whale: nerdctl network ls
List networks

### :whale: nerdctl network inspect
Display detailed information on one or more networks

:warning: The output format is not compatible with Docker.

### :whale: nerdctl network rm
Remove one or more networks

## Volume management
### :whale: nerdctl volume create
Create a volume

### :whale: nerdctl volume ls
List volumes

- :whale: `-q, --quiet`: Only display volume names

### :whale: nerdctl volume inspect
Display detailed information on one or more volumes

### :whale: nerdctl volume rm
Remove one or more volumes

## System
### :whale: nerdctl events
Get real time events from the server.

:warning: The output format is not compatible with Docker.

### :whale: nerdctl info
Display system-wide information

### :whale: nerdctl version
Show the nerdctl version information

## Global flags
- :nerd_face: `-a`, `--address`:  containerd address, optionally with "unix://" prefix
- :whale:     `-H`, `--host`: Docker-compatible alias for `-a`, `--address`
- :nerd_face: `-n`, `--namespace`: containerd namespace
- :nerd_face: `--snapshotter`: containerd snapshotter
- :nerd_face: `--cni-path`: CNI binary path (default: `/opt/cni/bin`) [`$CNI_PATH`]
- :nerd_face: `--cni-netconfpath`: CNI netconf path (default: `/etc/cni/net.d`) [`$NETCONFPATH`]
- :nerd_face: `--data-root`: nerdctl data root, e.g. "/var/lib/nerdctl"
- :nerd_face: `--cgroup-manager=(cgroupfs|systemd|none)`: cgroup manager
  - Default: "systemd" on cgroup v2 (rootful & rootless), "cgroupfs" on v1 rootful, "none" on v1 rootless
- :nerd_face: `--insecure-registry`: skips verifying HTTPS certs, and allows falling back to plain HTTP

## Unimplemented Docker commands
Container management:
- `docker attach`
- `docker cp`
- `docker diff`
- `docker rename`
- `docker wait`

- `docker container prune`

- `docker checkpoint *`

Stats:
- `docker stats`
- `docker top`

Image:
- `docker export` and `docker import`
- `docker history`
- `docker trust`

- `docker image prune`

- `docker manifest *`

Network management:
- `docker network connect`
- `docker network disconnect`
- `docker network prune`

Registry:
- `docker search`

Others:
- `docker context`
- Swarm commands are unimplemented and will not be implemented: `docker swarm|node|service|config|secret|stack *`
- Plugin commands are unimplemented and will not be implemented: `docker plugin *`

- - -

# Additional documents
- [`./docs/dir.md`](./docs/dir.md):           Directory layout (`/var/lib/nerdctl`)
- [`./docs/registry.md`](./docs/registry.md): Registry authentication (`~/.docker/config.json`)
- [`./docs/rootless.md`](./docs/rootless.md): Rootless mode
- [`./docs/stargz.md`](./docs/stargz.md):     Lazy-pulling using Stargz Snapshotter
- [`./docs/ocicrypt.md`](./docs/ocicrypt.md): Running encrypted images
