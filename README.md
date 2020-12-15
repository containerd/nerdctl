# nerdctl: Docker-compatible CLI for containerd

`nerdctl` is a Docker-compatible CLI for [contai**nerd**](https://containerd.io).

[![asciicast](https://asciinema.org/a/378377.svg)](https://asciinema.org/a/378377)


<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Examples](#examples)
- [Install](#install)
- [Motivation](#motivation)
- [Features present in `nerdctl` but not present in Docker](#features-present-in-nerdctl-but-not-present-in-docker)
- [Implementation status of Docker-compatible commands and flags](#implementation-status-of-docker-compatible-commands-and-flags)
- [Similar tools](#similar-tools)
- [Compiling nerdctl from source](#compiling-nerdctl-from-source)
- [Contributing to nerdctl](#contributing-to-nerdctl)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Examples

To run a container with the default CNI network (10.4.0.0/16):
```console
# nerdctl run -it --rm alpine
```

To build an image using BuildKit:
```console
# nerdctl build -t foo .
# nerdctl run -it --rm foo
```

To list Docker containers:
```console
# nerdctl --namespace moby ps -a
```

To list Kubernetes containers:
```console
# nerdctl --namespace k8s.io ps -a
```

## Install
Binaries are available for amd64, arm64, and arm-v7: https://github.com/AkihiroSuda/nerdctl/releases

In addition to containerd, the following components should be installed (optional):
- [CNI plugins](https://github.com/containernetworking/plugins): for using `nerdctl run`.
- [BuildKit](https://github.com/moby/buildkit): for using `nerdctl build`. BuildKit daemon (`buildkitd`) needs to be running.

## Motivation

The goal of `nerdctl` is to facilitate experimenting the cutting-edge features of containerd that are not present in Docker.

Such features includes, but not limited to, [lazy-pulling](./docs/stargz.md) and [encryption of images](https://github.com/containerd/imgcrypt).

Also, `nerdctl` might be potentially useful for debugging Kubernetes clusters, but it is not the primary goal.

## Features present in `nerdctl` but not present in Docker
- Namespacing as in `kubectl --namespace=<NS>`: `nerdctl --namespace=<NS> ps` .
- [Lazy-pulling using Stargz Snapshotter](./docs/stargz.md): `nerdctl --snapshotter=stargz run` .
- Exporting Docker/OCI dual-format archives: `nerdctl save` .
- Importing OCI archives: `nerdctl load` .
- Specifying a non-image rootfs: `nerdctl run -it --rootfs <ROOTFS> /bin/sh` . The CLI syntax conforms to Podman convention.

## Implementation status of Docker-compatible commands and flags

Run & Exec:
- `nerdctl run`
  - `-i`
  - `-t` (WIP: currently -t requires -i, and conflicts with -d)
  - `-d`
  - `--restart=(no|always)`
  - `--rm`
  - `--network=(bridge|host|none)`
  - `-p, --publish`
  - `--dns`
  - `-h, --hostname`
  - `--pull=(always|missing|never)`
  - `--cpus`
  - `--memory`
  - `--pids-limit`
  - `--cgroupns=(host|private)`
  - `-u, --user`
  - `--security-opt seccomp`
  - `--security-opt apparmor`
  - `--security-opt no-new-privileges`
  - `--privileged`
  - `--runtime`
  - `-v, --volume`
  - `--read-only`
  - `-w, --workdir`
  - `-e, --env`
  - `-l, --label`
  - `--label-file`

- `nerdctl exec`
  - `-i`
  - `-t` (WIP: currently -t requires -i, and conflicts with -d)
  - `-d`
  - `-w, --workdir`
  - `-e, --env`
  - `--privileged`

Container management:
- `nerdctl ps`
  - `-a, --all`: Show all containers (default shows just running)
  - `--no-trunc`: Don't truncate output
  - `-q, --quiet`: Only display container IDs

- `nerdctl logs`

- `nerdctl port`

- `nerdctl rm`
  - `-f`

- `nerdctl stop`

- `nerdctl kill`

- `nerdctl pause`

- `nerdctl unpause`

Build:
- `nerdctl build`
  - `-t, --tag`
  - `-f, --file`
  - `--target`
  - `--build-arg`
  - `--no-cache`
  - `--progress`
  - `--secret`
  - `--ssh`

- `nerdctl commit`
  - `-a, --author`
  - `-m, --message`

Image management:
- `nerdctl images`
  - `-q, --quiet`: Only show numeric IDs
  - `--no-trunc`: Don't truncate output

- `nerdctl pull`

- `nerdctl load`

- `nerdctl save`

- `nerdctl tag`

- `nerdctl rmi`

System:
- `nerdctl events`

- `nerdctl info`

- `nerdctl version`

Lots of commands and flags are currently missing. Pull requests are highly welcome.

## Similar tools

- `ctr`: incompatible with Docker, and not friendly to users
- [`crictl`](https://github.com/kubernetes-sigs/cri-tools): incompatible with Docker, not friendly to users, and does not support non-CRI features
- [k3c](https://github.com/rancher/k3c): needs an extra daemon, and does not support non-CRI features
- [PouchContainer](https://github.com/alibaba/pouch): needs an extra daemon

## Compiling nerdctl from source

Run `make && sudo make install`.

Using `go get github.com/AkihiroSuda/nerdctl` is possible, but unrecommended because it does not fill version strings printed in `nerdctl version`

## Contributing to nerdctl

- Please certify your [Developer Certificate of Origin (DCO)](https://developercertificate.org/), by signing off your commit with `git commit -s` and with your real name.
