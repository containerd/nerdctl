# Darwin support


| :zap:        Darwin runtimes are at the very early stage of development |
|--------------------------------------------------------------------------|

nerdctl provides experimental support for running containers on macOS hosts.

## Installation

On darwin, [runu](https://github.com/ukontainer/runu) is only available runtime implementation for now.
Below is the list of required installation before using nerdctl on darwin.

- containerd (version 1.6.0-rc? or later)
- darwin snapshotter
 - proxy snapshotter `go install github.com/ukontainer/darwin-snapshotter/cmd/containerd-darwin-snapshotter-grpc`
 - mount helper `go install github.com/ukontainer/darwin-snapshotter/cmd/mount_containerd_darwin`
- runu
 - runtime `go install github.com/ukontainer/runu`
 - shim `go install github.com/ukontainer/runu/cmd/containerd-shim-runu-v1`

## Usage

You can use custom images of ukontainer, available at https://github.com/orgs/ukontainer/packages .

```sh
nerdctl run -i --net none --snapshotter=sparsebundle ghcr.io/ukontainer/runu-base:0.7 hello
```
## Difference with `lima nerdctl`

There is a way to use `nerdctl` on macOS, using [Lima](https://github.com/lima-vm/lima).  Lima offers us to
use Linux containers via a Linux VM instance, and we can execute nerdctl from macOS.

The darwin support of nerdctl takes a different approach: execute all programs as macOS processes, instead of
executing processes on a Linux VM.

## Limitations & Bugs

- :warning: CNI & CNI plugins are not yet ported to darwin. The only supported
  network type is `none`
