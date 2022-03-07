# FreeBSD


| :zap:        FreeBSD runtimes are at the very early stage of development |
|--------------------------------------------------------------------------|

nerdctl provides experimental support for running FreeBSD jails on FreeBSD hosts.

## Installation

You will need the most up-to-date containerd build along with a containerd shim,
such as [runj](https://github.com/samuelkarp/runj). Follow the build
instructions in the respective repositories.

## Usage

You can use the `knast/freebsd` image to run a standard FreeBSD 13 jail:

```sh
nerdctl run --net none -it knast/freebsd:13-STABLE
```

:warning: `nerdctl run` has been broken on FreeBSD (FIXME): https://github.com/containerd/nerdctl/issues/868

## Limitations & Bugs

- :warning: CNI & CNI plugins are not yet ported to FreeBSD. The only supported
  network type is `none`
- :warning: buildkit is not yet ported to FreeBSD.
  - [ ] https://github.com/tonistiigi/fsutil/pull/109 - buildkit dependency
  - [ ] https://github.com/moby/moby/pull/42866 - buildkit dependency
- :warning: Linuxulator containers support is
  WIP. https://github.com/containerd/nerdctl/issues/280 https://github.com/containerd/containerd/pull/5480

- :bug: `nerdctl compose` commands currently don't work. https://github.com/containerd/containerd/pull/5991
