# FreeBSD


| :zap:        FreeBSD runtimes are at the very early stage of development |
|--------------------------------------------------------------------------|

nerdctl provides experimental support for running FreeBSD jails on FreeBSD hosts.

## Installation

You will need the most up-to-date containerd build along with a containerd shim,
such as [runj](https://github.com/samuelkarp/runj). Follow the build
instructions in the respective repositories.

## Usage

You can use the `dougrabson/freebsd13.2-small` image to run a FreeBSD 13 jail:

```sh
nerdctl run --net none -it dougrabson/freebsd13.2-small
```

Alternatively use `--platform` parameter to run linux containers

```sh
nerdctl run --platform linux --net none -it amazonlinux:2
```


## Limitations & Bugs

- :warning: CNI & CNI plugins are not yet ported to FreeBSD. The only supported
  network type is `none`
