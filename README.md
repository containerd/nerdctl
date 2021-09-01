[[⬇️ **Download]**](https://github.com/containerd/nerdctl/releases)
[[📖 **Command reference]**](#command-reference)
[[📚 **Additional documents]**](#additional-documents)

# nerdctl: Docker-compatible CLI for containerd

`nerdctl` is a Docker-compatible CLI for [contai**nerd**](https://containerd.io).

 ✅ Same UI/UX as `docker`

 ✅ Supports Docker Compose (`nerdctl compose up`)

 ✅ Supports [rootless mode](./docs/rootless.md)

 ✅ Supports [lazy-pulling (Stargz)](./docs/stargz.md)

 ✅ Supports [encrypted images (ocicrypt)](./docs/ocicrypt.md)

nerdctl is a **non-core** sub-project of containerd.

## Examples

### Basic usage

To run a container with the default CNI network (10.4.0.0/24):
```console
# nerdctl run -it --rm alpine
```

To build an image using BuildKit:
```console
# nerdctl build -t foo /some-dockerfile-directory
# nerdctl run -it --rm foo
```

To run containers from `docker-compose.yaml`:
```console
# nerdctl compose -f ./examples/compose-wordpress/docker-compose.yaml up
```

See also [`./examples/compose-wordpress`](./examples/compose-wordpress).

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
Binaries are available here: https://github.com/containerd/nerdctl/releases

In addition to containerd, the following components should be installed (optional):
- [CNI plugins](https://github.com/containernetworking/plugins): for using `nerdctl run`.
- [CNI isolation plugin](https://github.com/AkihiroSuda/cni-isolation): for isolating bridge networks (`nerdctl network create`)
- [BuildKit](https://github.com/moby/buildkit): for using `nerdctl build`. BuildKit daemon (`buildkitd`) needs to be running.
- [RootlessKit](https://github.com/rootless-containers/rootlesskit) and [slirp4netns](https://github.com/rootless-containers/slirp4netns): for [Rootless mode](./docs/rootless.md)
   - RootlessKit needs to be v0.10.0 or later. v0.14.1 or later is recommended.
   - slirp4netns needs to be v0.4.0 or later. v1.1.7 or later is recommended.

These dependencies are included in `nerdctl-full-<VERSION>-<OS>-<ARCH>.tar.gz`, but not included in `nerdctl-<VERSION>-<OS>-<ARCH>.tar.gz`.

### macOS

[Lima](https://github.com/AkihiroSuda/lima) project provides Linux virtual machines with built-in integration for nerdctl.

```console
$ brew install lima
$ limactl start
$ lima nerdctl run -d --name nginx -p 127.0.0.1:8080:80 nginx:alpine
```

NOTE: ARM Mac requires installing a patched version of QEMU, see [Lima](https://github.com/AkihiroSuda/lima) documentation.

### Windows

- Linux containers: Known to work on WSL2
- Windows containers: WIP, see [PR #197](https://github.com/containerd/nerdctl/pull/197)

### Docker

To run containerd and nerdctl inside Docker:
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
- Connecting a container to multiple networks at once: `nerdctl run --net foo --net bar`

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
nerdctl is a containerd **non-core** sub-project, licensed under the [Apache 2.0 license](./LICENSE).
As a containerd non-core sub-project, you will find the:
 * [Project governance](https://github.com/containerd/project/blob/master/GOVERNANCE.md),
 * [Maintainers](./MAINTAINERS),
 * and [Contributing guidelines](https://github.com/containerd/project/blob/master/CONTRIBUTING.md)

information in our [`containerd/project`](https://github.com/containerd/project) repository.

### Compiling nerdctl from source

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
Run `go test -exec sudo -v ./cmd/nerdctl/... -args test.target=docker` to ensure that the test suite is compatible with Docker.

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
    - [:whale: nerdctl wait](#whale-nerdctl-wait)
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
    - [:whale: nerdctl image inspect](#whale-nerdctl-image-inspect)
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
  - [Namespace management](#namespace-management)
    - [:nerd_face: nerdctl namespace ls](#nerd_face-nerdctl-namespace-ls)
  - [System](#system)
    - [:whale: nerdctl events](#whale-nerdctl-events)
    - [:whale: nerdctl info](#whale-nerdctl-info)
    - [:whale: nerdctl version](#whale-nerdctl-version)
  - [Stats](#stats)
    - [:whale: nerdctl top](#whale-nerdctl-top)
  - [Shell completion](#shell-completion)
    - [:nerd_face: nerdctl completion bash](#nerd_face-nerdctl-completion-bash)
  - [Compose](#compose)
    - [:whale: nerdctl compose](#whale-nerdctl-compose)
    - [:whale: nerdctl compose up](#whale-nerdctl-compose-up)
    - [:whale: nerdctl compose logs](#whale-nerdctl-compose-logs)
    - [:whale: nerdctl compose build](#whale-nerdctl-compose-build)
    - [:whale: nerdctl compose down](#whale-nerdctl-compose-down)
  - [Global flags](#global-flags)
  - [Unimplemented Docker commands](#unimplemented-docker-commands)
- [Additional documents](#additional-documents)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->


 
## Run & Exec
### :whale: nerdctl run
Run a command in a new container.

Usage: `nerdctl run [OPTIONS] IMAGE [COMMAND] [ARG...]`

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
- :whale: `--pid=(host)`: PID namespace to use

Network flags:
- :whale: `--net, --network=(bridge|host|none|<CNI>)`: Connect a container to a network
  - Default: "bridge"
  - :nerd_face: Unlike Docker, this flag can be specified multiple times (`--net foo --net bar`)
- :whale: `-p, --publish`: Publish a container's port(s) to the host
- :whale: `--dns`: Set custom DNS servers
- :whale: `-h, --hostname`: Container host name

Cgroup flags:
- :whale: `--cpus`: Number of CPUs
- :whale: `--cpu-shares`: CPU shares (relative weight)
- :whale: `--cpuset-cpus`: CPUs in which to allow execution (0-3, 0,1)
- :whale: `--memory`: Memory limit
- :whale: `--pids-limit`: Tune container pids limit
- :whale: `--cgroupns=(host|private)`: Cgroup namespace to use
  - Default: "private" on cgroup v2 hosts, "host" on cgroup v1 hosts
- :whale: `--device`: Add a host device to the container

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
- :whale: `--sysctl`: Sysctl options, e.g \"net.ipv4.ip_forward=1\"

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
- :whale: `--env-file`: Set environment variables from file

Metadata flags:
- :whale: `--name`: Assign a name to the container
- :whale: `-l, --label`: Set meta data on a container
- :whale: `--label-file`: Read in a line delimited file of labels
- :whale: `--cidfile`: Write the container ID to the file
- :nerd_face: `--pidfile`: file path to write the task's pid. The CLI syntax conforms to Podman convention.

Shared memory flags:
- :whale: `--shm-size`: Size of `/dev/shm`

GPU flags:
- :whale: `--gpus`: GPU devices to add to the container ('all' to pass all GPUs). Please see also [./docs/gpu.md](./docs/gpu.md) for details.

Other `docker run` flags are on plan but unimplemented yet.
<details>
<summary> Clicke here to show all the `docker run` flags (Docker 20.10)</summary>

<p>

```
Usage:  docker run [OPTIONS] IMAGE [COMMAND] [ARG...]

Run a command in a new container

Options:
      --add-host list                  Add a custom host-to-IP mapping (host:ip)
  -a, --attach list                    Attach to STDIN, STDOUT or STDERR
      --blkio-weight uint16            Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)
      --blkio-weight-device list       Block IO weight (relative device weight) (default [])
      --cap-add list                   Add Linux capabilities
      --cap-drop list                  Drop Linux capabilities
      --cgroup-parent string           Optional parent cgroup for the container
      --cgroupns string                Cgroup namespace to use (host|private)
                                       'host':    Run the container in the Docker host's cgroup namespace
                                       'private': Run the container in its own private cgroup namespace
                                       '':        Use the cgroup namespace as configured by the
                                                  default-cgroupns-mode option on the daemon (default)
      --cidfile string                 Write the container ID to the file
      --cpu-period int                 Limit CPU CFS (Completely Fair Scheduler) period
      --cpu-quota int                  Limit CPU CFS (Completely Fair Scheduler) quota
      --cpu-rt-period int              Limit CPU real-time period in microseconds
      --cpu-rt-runtime int             Limit CPU real-time runtime in microseconds
  -c, --cpu-shares int                 CPU shares (relative weight)
      --cpus decimal                   Number of CPUs
      --cpuset-cpus string             CPUs in which to allow execution (0-3, 0,1)
      --cpuset-mems string             MEMs in which to allow execution (0-3, 0,1)
  -d, --detach                         Run container in background and print container ID
      --detach-keys string             Override the key sequence for detaching a container
      --device list                    Add a host device to the container
      --device-cgroup-rule list        Add a rule to the cgroup allowed devices list
      --device-read-bps list           Limit read rate (bytes per second) from a device (default [])
      --device-read-iops list          Limit read rate (IO per second) from a device (default [])
      --device-write-bps list          Limit write rate (bytes per second) to a device (default [])
      --device-write-iops list         Limit write rate (IO per second) to a device (default [])
      --disable-content-trust          Skip image verification (default true)
      --dns list                       Set custom DNS servers
      --dns-option list                Set DNS options
      --dns-search list                Set custom DNS search domains
      --domainname string              Container NIS domain name
      --entrypoint string              Overwrite the default ENTRYPOINT of the image
  -e, --env list                       Set environment variables
      --env-file list                  Read in a file of environment variables
      --expose list                    Expose a port or a range of ports
      --gpus gpu-request               GPU devices to add to the container ('all' to pass all GPUs)
      --group-add list                 Add additional groups to join
      --health-cmd string              Command to run to check health
      --health-interval duration       Time between running the check (ms|s|m|h) (default 0s)
      --health-retries int             Consecutive failures needed to report unhealthy
      --health-start-period duration   Start period for the container to initialize before starting health-retries countdown (ms|s|m|h) (default 0s)
      --health-timeout duration        Maximum time to allow one check to run (ms|s|m|h) (default 0s)
      --help                           Print usage
  -h, --hostname string                Container host name
      --init                           Run an init inside the container that forwards signals and reaps processes
  -i, --interactive                    Keep STDIN open even if not attached
      --ip string                      IPv4 address (e.g., 172.30.100.104)
      --ip6 string                     IPv6 address (e.g., 2001:db8::33)
      --ipc string                     IPC mode to use
      --isolation string               Container isolation technology
      --kernel-memory bytes            Kernel memory limit
  -l, --label list                     Set meta data on a container
      --label-file list                Read in a line delimited file of labels
      --link list                      Add link to another container
      --link-local-ip list             Container IPv4/IPv6 link-local addresses
      --log-driver string              Logging driver for the container
      --log-opt list                   Log driver options
      --mac-address string             Container MAC address (e.g., 92:d0:c6:0a:29:33)
  -m, --memory bytes                   Memory limit
      --memory-reservation bytes       Memory soft limit
      --memory-swap bytes              Swap limit equal to memory plus swap: '-1' to enable unlimited swap
      --memory-swappiness int          Tune container memory swappiness (0 to 100) (default -1)
      --mount mount                    Attach a filesystem mount to the container
      --name string                    Assign a name to the container
      --network network                Connect a container to a network
      --network-alias list             Add network-scoped alias for the container
      --no-healthcheck                 Disable any container-specified HEALTHCHECK
      --oom-kill-disable               Disable OOM Killer
      --oom-score-adj int              Tune host's OOM preferences (-1000 to 1000)
      --pid string                     PID namespace to use
      --pids-limit int                 Tune container pids limit (set -1 for unlimited)
      --platform string                Set platform if server is multi-platform capable
      --privileged                     Give extended privileges to this container
  -p, --publish list                   Publish a container's port(s) to the host
  -P, --publish-all                    Publish all exposed ports to random ports
      --pull string                    Pull image before running ("always"|"missing"|"never") (default "missing")
      --read-only                      Mount the container's root filesystem as read only
      --restart string                 Restart policy to apply when a container exits (default "no")
      --rm                             Automatically remove the container when it exits
      --runtime string                 Runtime to use for this container
      --security-opt list              Security Options
      --shm-size bytes                 Size of /dev/shm
      --sig-proxy                      Proxy received signals to the process (default true)
      --stop-signal string             Signal to stop a container (default "SIGTERM")
      --stop-timeout int               Timeout (in seconds) to stop a container
      --storage-opt list               Storage driver options for the container
      --sysctl map                     Sysctl options (default map[])
      --tmpfs list                     Mount a tmpfs directory
  -t, --tty                            Allocate a pseudo-TTY
      --ulimit ulimit                  Ulimit options (default [])
  -u, --user string                    Username or UID (format: <name|uid>[:<group|gid>])
      --userns string                  User namespace to use
      --uts string                     UTS namespace to use
  -v, --volume list                    Bind mount a volume
      --volume-driver string           Optional volume driver for the container
      --volumes-from list              Mount volumes from the specified container(s)
  -w, --workdir string                 Working directory inside the container
```

</p>

</details>

### :whale: nerdctl exec
Run a command in a running container.

Usage: `nerdctl exec [OPTIONS] CONTAINER COMMAND [ARG...]`

Flags:
- :whale: `-i, --interactive`: Keep STDIN open even if not attached
- :whale: `-t, --tty`: Allocate a pseudo-TTY
  - :warning: WIP: currently `-t` requires `-i`, and conflicts with `-d`
- :whale: `-d, --detach`: Detached mode: run command in the background
- :whale: `-w, --workdir`: Working directory inside the container
- :whale: `-e, --env`: Set environment variables
- :whale: `--privileged`: Give extended privileges to the command

Unimplemented `docker exec` flags: `--detach-keys`, `--env-file`, `--user`

## Container management
### :whale: nerdctl ps
List containers.

Usage: `nerdctl ps [OPTIONS]`

Flags:
- :whale: `-a, --all`: Show all containers (default shows just running)
- :whale: `--no-trunc`: Don't truncate output
- :whale: `-q, --quiet`: Only display container IDs
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

Unimplemented `docker ps` flags: `--filter`, `--last`, `--size`

### :whale: nerdctl inspect
Display detailed information on one or more containers.

Usage: `nerctl inspect [OPTIONS] NAME|ID [NAME|ID...]`

Flags:
- :nerd_face: `--mode=(dockercompat|native)`: Inspection mode. "native" produces more information.
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

Unimplemented `docker inspect` flags:  `--size`, `--type`

### :whale: nerdctl logs
Fetch the logs of a container. 

:warning: Currently, only containers created with `nerdctl run -d` are supported.

Usage: `nerctl logs [OPTIONS] CONTAINER`

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

Unimplemented `docker stop` flags: `--time`

### :whale: nerdctl start
Start one or more running containers.

Usage: `nerdctl start [OPTIONS] CONTAINER [CONTAINER...]`

Unimplemented `docker start` flags: `--attach`, `--checkpoint`, `--checkpoint-dir`, `--detach-keys`, `--interactive`

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

## Build
### :whale: nerdctl build
Build an image from a Dockerfile.

:information_source: Needs buildkitd to be running.

Usage: `nerdctl build [OPTIONS] PATH`

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

Unimplemented `docker build` flags: `--add-host`, `--cache-from`, `--iidfile`, `--label`, `--network`, `--platform`, `--quiet`, `--squash`

### :whale: nerdctl commit
Create a new image from a container's changes

Usage: `nerdctl commit [OPTIONS] CONTAINER [REPOSITORY[:TAG]]`

Flags:
- :whale: `-a, --author`: Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")
- :whale: `-m, --message`: Commit message

Unimplemented `docker commit` flags: `--change`, `--pause`

## Image management

### :whale: nerdctl images
List images

Usage: `nerdctl images [OPTIONS] [REPOSITORY[:TAG]]`

Flags:
- :whale: `-q, --quiet`: Only show numeric IDs
- :whale: `--no-trunc`: Don't truncate output
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

Unimplemented `docker images` flags: `--all`, `--digests`, `--filter`

### :whale: nerdctl pull
Pull an image from a registry.

Usage: `nerdctl pull [OPTIONS] NAME[:TAG|@DIGEST]`

Unimplemented `docker pull` flags: `--all-tags`, `--disable-content-trust` (default true), `--platform`, `--quiet`

### :whale: nerdctl push
Push an image to a registry.

Usage: `nerdctl push [OPTIONS] NAME[:TAG]`

Unimplemented `docker push` flags: `--all-tags`, `--disable-content-trust` (default true), `--quiet`

### :whale: nerdctl load
Load an image from a tar archive or STDIN.

:nerd_face: Supports both Docker Image Spec v1.2 and OCI Image Spec v1.0.

Usage: `nerdctl load [OPTIONS]`

Flags:
- :whale: `-i, --input`: Read from tar archive file, instead of STDIN

Unimplemented `docker load` flags: `--quiet`

### :whale: nerdctl save
Save one or more images to a tar archive (streamed to STDOUT by default)

:nerd_face: The archive implements both Docker Image Spec v1.2 and OCI Image Spec v1.0.

Usage: `nerdctl save [OPTIONS] IMAGE [IMAGE...]`

Flags:
- :whale: `-o, --output`: Write to a file, instead of STDOUT

### :whale: nerdctl tag
Create a tag TARGET\_IMAGE that refers to SOURCE\_IMAGE.

Usage: `nerdctl tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

### :whale: nerdctl rmi
Remove one or more images

Usage: `nerdctl rmi [OPTIONS] IMAGE [IMAGE...]`

Unimplemented `docker rmi` flags: `--force`, `--no-prune`

### :whale: nerdctl image inspect
Display detailed information on one or more images.

Usage: `nerctl image inspect [OPTIONS] NAME|ID [NAME|ID...]`

Flags:
- :nerd_face: `--mode=(dockercompat|native)`: Inspection mode. "native" produces more information.
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

### :nerd_face: nerdctl image convert
Convert an image format.

e.g., `nerdctl image convert --estargz --oci example.com/foo:orig example.com/foo:esgz`

Usage: `nerdctl image convert [OPTIONS] SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

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

Usage: `nerdctl login [OPTIONS] [SERVER]`

Flags:
- :whale: `-u, --username`:   Username
- :whale: `-p, --password`:   Password
- :whale: `--password-stdin`: Take the password from stdin

### :whale: nerdctl logout
Log out from a Docker registry

Usage: `nerdctl logout [SERVER]`

## Network management
### :whale: nerdctl network create
Create a network

:information_source: To isolate CNI bridge, [CNI isolation plugin](https://github.com/AkihiroSuda/cni-isolation) needs to be installed.

Usage: `nerdctl network create [OPTIONS] NETWORK`

Flags:
- :whale: `--subnet`: Subnet in CIDR format that represents a network segment, e.g. "10.5.0.0/16" 
- :whale: `--label`: Set metadata on a network

Unimplemented `docker network create` flags: `--attachable`, `--aux-address`, `--config-from`, `--config-only`, `--driver`, `--gateway`, `--ingress`, `--internal`, `--ip-range`, `--ipam-driver`, `--ipam-opt`, `--ipv6`, `--opt`, `--scope`

### :whale: nerdctl network ls
List networks

Usage: `nerdctl network ls [OPTIONS]`

Flags:
- :whale: `-q, --quiet`: Only display network IDs
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

Unimplemented `docker network ls` flags: `--filter`, `--no-trunc`

### :whale: nerdctl network inspect
Display detailed information on one or more networks

Usage: `nerdctl network inspect [OPTIONS] NETWORK [NETWORK...]`

Unimplemented `docker network inspect` flags: `--format`, `--verbose`

### :whale: nerdctl network rm
Remove one or more networks

Usage: `nerdctl network rm NETWORK [NETWORK...]`

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
- :whale: `--format`: Format the output using the given Go template, e.g, `{{json .}}`

Unimplemented `docker volume ls` flags: `--filter`

### :whale: nerdctl volume inspect
Display detailed information on one or more volumes

Usage: `nerdctl volume inspect [OPTIONS] VOLUME [VOLUME...]`

Unimplemented `docker volume inspect` flags: `--format`

### :whale: nerdctl volume rm
Remove one or more volumes

Usage: `nerdctl volume rm [OPTIONS] VOLUME [VOLUME...]`

- :whale: `-f, --force`: Force the removal of one or more volumes
  - :warning: WIP: currently, volumes are always forcibly removed, even when `--force` is not specified.

## Namespace management

### :nerd_face: nerdctl namespace ls
List containerd namespaces such as "default", "moby", or "k8s.io".

Usage: `nerdctl namespace ls [OPTIONS]`

Flags:
- `-q, --quiet`: Only display namespace names

## System
### :whale: nerdctl events
Get real time events from the server.

:warning: The output format is not compatible with Docker.

Usage: `nerdctl events [OPTIONS]`

Unimplemented `docker events` flags: `--filter`, `--format`, `--since`, `--until`

### :whale: nerdctl info
Display system-wide information

Usage: `nerdctl info [OPTIONS]`

Flags:
- :whale: `-f, --format`: Format the output using the given Go template, e.g, `{{json .}}`

### :whale: nerdctl version
Show the nerdctl version information

Usage: `nerdctl version [OPTIONS]`

Flags:
- :whale: `-f, --format`: Format the output using the given Go template, e.g, `{{json .}}`

## Stats
### :whale: nerdctl top
Display the running processes of a container.


Usage: `nerdctl top CONTAINER [ps OPTIONS]`


## Shell completion

### :nerd_face: nerdctl completion bash
Show bash completion.

Usage: add the following line to `~/.bash_profile`:
```bash
source <(nerdctl completion bash)
```

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
- :whale: `--no-color`: Produce monochrome output
- :whale: `--no-log-prefix`: Don't print prefix in logs
- :whale: `--build`: Build images before starting containers.

Unimplemented `docker-compose up` flags: `--quiet-pull`, `--no-deps`, `--force-recreate`, `--always-recreate-deps`, `--no-recreate`,
`--no-start`, `--abort-on-container-exit`, `--attach-dependencies`, `--timeout`, `--renew-anon-volumes`, `--remove-orphans`, `--exit-code-from`,
`--scale`

### :whale: nerdctl compose logs
Create and start containers

Usage: `nerdctl compose logs [OPTIONS]`

Flags:
- :whale: `--no-color`: Produce monochrome output
- :whale: `--no-log-prefix`: Don't print prefix in logs
- :whale: `--timestamps`: Show timestamps
- :whale: `--tail`: Number of lines to show from the end of the logs


### :whale: nerdctl compose build
Build or rebuild services.

Usage: `nerdctl compose build [OPTIONS]`

Flags:
- :whale: `--build-arg`: Set build-time variables for services
- :whale: `--no-cache`: Do not use cache when building the image
- :whale: `--progress`: Set type of progress output (auto, plain, tty)

Unimplemented `docker-compose build` flags:  `--compress`, `--force-rm`, `--memory`, `--no-rm`, `--parallel`, `--pull`, `--quiet`

### :whale: nerdctl compose down
Remove containers and associated resources

Usage: `nerdctl compose up [OPTIONS] [SERVICE...]`

Flags:
- :whale: `-v, --volumes`: Remove named volumes declared in the volumes section of the Compose file and anonymous volumes attached to containers

Unimplemented `docker-compose down` flags: `--rmi`, `--remove-orphans`, `--timeout`

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
- `docker create`
- `docker attach`
- `docker cp`
- `docker diff`
- `docker rename`

- `docker container prune`

- `docker checkpoint *`

Stats:
- `docker stats`

Image:
- `docker export` and `docker import`
- `docker history`

- `docker image prune`

- `docker trust *`
- `docker manifest *`

Network management:
- `docker network connect`
- `docker network disconnect`
- `docker network prune`

Registry:
- `docker search`

Compose:
- `docker-compose config|create|events|exec|images|kill|pause|port|ps|pull|push|restart|rm|run|scale|start|stop|top|unpause`

Others:
- `docker system df`
- `docker system prune`
- `docker context`
- Swarm commands are unimplemented and will not be implemented: `docker swarm|node|service|config|secret|stack *`
- Plugin commands are unimplemented and will not be implemented: `docker plugin *`

- - -

# Additional documents
- [`./docs/compose.md`](./docs/compose.md):   Compose
- [`./docs/dir.md`](./docs/dir.md):           Directory layout (`/var/lib/nerdctl`)
- [`./docs/gpu.md`](./docs/gpu.md):            Using GPUs inside containers
- [`./docs/registry.md`](./docs/registry.md): Registry authentication (`~/.docker/config.json`)
- [`./docs/rootless.md`](./docs/rootless.md): Rootless mode
- [`./docs/stargz.md`](./docs/stargz.md):     Lazy-pulling using Stargz Snapshotter
- [`./docs/ocicrypt.md`](./docs/ocicrypt.md): Running encrypted images
