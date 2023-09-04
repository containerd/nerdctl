# FreeBSD


| :zap:        FreeBSD runtimes are at the very early stage of development |
|--------------------------------------------------------------------------|

nerdctl provides experimental support for running FreeBSD jails on FreeBSD hosts.

## Installation

You will need the most up-to-date containerd build along with a containerd shim,
such as [runj](https://github.com/samuelkarp/runj). Follow the build
instructions in the respective repositories.

The runj runtime must be configured to create vnet jails by default. To do this, you need to create

`/etc/nerdctl/runj.ext.json` with the following content.

```
{
  "network": {
    "vnet": {
      "mode": "new"
    }
  }
}
```

## Usage

You can use the `dougrabson/freebsd13.2-small` image to run a FreeBSD 13 jail:

```sh
nerdctl run --net=none -it dougrabson/freebsd13.2-small
```

Alternatively use `--platform` parameter to run linux containers

```sh
nerdctl run --platform linux --net=none -it amazonlinux:2
```

:warning: running linux containers requires `linux64` module loaded:

```
kldload linux64
```


## CNI networking

| :construction:        CNI networking requires host OS to be version 13.3 and higher. Lower versions are not guaranteed to work. |
|---------------------------------------------------------------------------------------------------------------------------------|

CNI plugins can be installed from the repository

```sh
pkg install net/containernetworking-plugins
```

You can then drop the `--net=none` flag and run commands as usual.

```sh
nerdctl run -it dougrabson/freebsd13.2-small ping 1.1.1.1
```
