# nerdctl: Docker-compatible CLI for containerd

`nerdctl` is a Docker-compatible CLI for [containerd](https://containerd.io).

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

```console
# go get github.com/AkihiroSuda/nerdctl
```

In addition to containerd, the following components should be installed (optional):
- [CNI plugins](https://github.com/containernetworking/plugins): for internet connectivity.
- [BuildKit](https://github.com/moby/buildkit): for using `nerdctl build`. BuildKit daemon (`buildkitd`) needs to be running.

## Motivation

The goal of `nerdctl` is to facilitate experimenting the cutting-edge features of containerd that are not present in Docker.

Such features includes, but not limited to, [lazy-pulling](https://github.com/containerd/stargz-snapshotter) and [encryption of images](https://github.com/containerd/imgcrypt).

Also, `nerdctl` might be potentially useful for debugging Kubernetes clusters, but it is not the primary goal.

## Similar tools

- `ctr`: incompatible with Docker, and not friendly to users
- [`crictl`](https://github.com/kubernetes-sigs/cri-tools): incompatible with Docker, not friendly to users, and does not support non-CRI features
- [k3c](https://github.com/rancher/k3c): needs an extra daemon, and does not support non-CRI features
- [PouchContainer](https://github.com/alibaba/pouch): needs an extra daemon

## Implementation status of Docker-compatible commands and flags

- `nerdctl build`
  - `-t`

- `nerdctl ps`
  - `-a` (WIP: Ignored and always assumed to be true)
  - `--no-trunc`

- `nerdctl pull`

- `nerdctl run`
  - `-i` (WIP: always needs to be true)
  - `-t` (WIP: always needs to be true)
  - `--rm`
  - `--network=(bridge|host|none)`
  - `--dns`
  - `--pull=(always|missing|never)`

Lots of commands and flags are currently missing. Pull requests are highly welcome.

## Contributing to nerdctl

- Please certify your [Developer Certificate of Origin (DCO)](https://developercertificate.org/), by signing off your commit with `git commit -s` and with your real name.
