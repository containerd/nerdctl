# Distribute Container Images on IPFS (Experimental)

| :zap: Requirement | nerdctl >= 0.14 |
|-------------------|-----------------|

You can distribute container images without registries, using IPFS.

IPFS support is completely optional. Your host is NOT connected to any P2P network, unless you opt in to [install and run IPFS daemon](https://docs.ipfs.io/install/).

## Prerequisites

### ipfs daemon

Make sure an IPFS daemon such as [Kubo](https://github.com/ipfs/kubo) (former go-ipfs) is running on your host.
For example, you can run Kubo using the following command.

```
ipfs daemon
```

In rootless mode, you need to install ipfs daemon using `containerd-rootless-setuptool.sh`.

```
containerd-rootless-setuptool.sh -- install-ipfs --init
```

> NOTE: correctly set IPFS_PATH as described in the output of the above command.

:information_source: If you want to expose some ports of ipfs daemon (e.g. 4001), you can install rootless containerd using `containerd-rootless-setuptool.sh install` with `CONTAINERD_ROOTLESS_ROOTLESSKIT_FLAGS="--publish=0.0.0.0:4001:4001/tcp"` environment variable.

:information_source: If you don't want IPFS to communicate with nodes on the internet, you can run IPFS daemon in offline mode using `--offline` flag or you can create a private IPFS network as described [here](https://github.com/containerd/stargz-snapshotter/blob/main/docs/ipfs.md#appendix-1-creating-ipfs-private-network).

:information_source: Instead of locally launching IPFS daemon, you can specify the address of the IPFS API using `--ipfs-address` flag.

## IPFS-enabled image and OCI Compatibility

Image distribution on IPFS is achieved by OCI-compatible *IPFS-enabled image format*.
nerdctl automatically converts an image to IPFS-enabled when necessary.
For example, when nerdctl pushes an image to IPFS, if that image isn't an IPFS-enabled one, it converts that image to the IPFS-enabled one.

Please see [the doc in stargz-snapshotter project](https://github.com/containerd/stargz-snapshotter/blob/v0.10.0/docs/ipfs.md) for details about IPFS-enabled image format.

## Using nerdctl with IPFS

nerdctl supports an image name prefix `ipfs://` to handle images on IPFS.

### `nerdctl push ipfs://<image-name>`

For `nerdctl push`, you can specify `ipfs://` prefix for arbitrary image names stored in containerd.
When this prefix is specified, nerdctl pushes that image to IPFS.

```console
> nerdctl push ipfs://ubuntu:20.04
INFO[0000] pushing image "ubuntu:20.04" to IPFS
INFO[0000] ensuring image contents
bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze
```

At last line of the output, the IPFS CID of the pushed image is printed.
You can use this CID to pull this image from IPFS.

You can also specify `--estargz` option to enable [eStargz-based lazy pulling](https://github.com/containerd/stargz-snapshotter/blob/v0.10.0/docs/ipfs.md) on IPFS.
Please see the later section for details.

```console
> nerdctl push --estargz ipfs://fedora:36
INFO[0000] pushing image "fedora:36" to IPFS
INFO[0000] ensuring image contents
INFO[0011] converted "application/vnd.docker.image.rootfs.diff.tar.gzip" to sha256:cd4be969f12ef45dee7270f3643f796364045edf94cfa9ef6744d91d5cdf2208
bafkreibp2ncujcia663uum25ustwvmyoguxqyzjnxnlhebhsgk2zowscye
```

### `nerdctl pull ipfs://<CID>` and `nerdctl run ipfs://<CID>`

You can pull an image from IPFS by specifying `ipfs://<CID>` where `CID` is the CID of the image.

```console
> nerdctl pull ipfs://bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze
bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze:                      resolved       |++++++++++++++++++++++++++++++++++++++|
index-sha256:28bfa1fc6d491d3bee91bab451cab29c747e72917efacb0adc4e73faffe1f51c:    done           |++++++++++++++++++++++++++++++++++++++|
manifest-sha256:f6eed19a2880f1000be1d46fb5d114d094a59e350f9d025580f7297c8d9527d5: done           |++++++++++++++++++++++++++++++++++++++|
config-sha256:ba6acccedd2923aee4c2acc6a23780b14ed4b8a5fa4e14e252a23b846df9b6c1:   done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:7b1a6ab2e44dbac178598dabe7cff59bd67233dba0b27e4fbd1f9d4b3c877a54:    done           |++++++++++++++++++++++++++++++++++++++|
elapsed: 1.2 s                                                                    total:  27.2 M (22.7 MiB/s)
```

`nerdctl run` also supports the same image name syntax.
When specified, this command pulls the image from IPFS.

```console
> nerdctl run --rm -it ipfs://bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze echo hello
hello
```

You can also push that image to the container registry.

```
nerdctl tag ipfs://bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze ghcr.io/ktock/ubuntu:20.04-ipfs
nerdctl push ghcr.io/ktock/ubuntu:20.04-ipfs
```

The pushed image can run on other (IPFS-agnostic) runtimes.

```console
> docker run --rm -it ghcr.io/ktock/ubuntu:20.04-ipfs echo hello
hello
```

:information_source: Note that though the IPFS-enabled image is OCI compatible, some runtimes including [containerd](https://github.com/containerd/containerd/pull/6221) and [podman](https://github.com/containers/image/pull/1403) had bugs and failed to pull that image. Containerd fixed this since v1.5.8, podman fixed this since commit [`b55fb86c28b7d743cf59701332cd78d4294c7c54`](https://github.com/containers/image/commit/b55fb86c28b7d743cf59701332cd78d4294c7c54).

### `nerdctl build` and `localhost:5050/ipfs/<CID>` image reference

You can build images using base images on IPFS.
BuildKit >= v0.9.3 is needed.

In Dockerfile, instead of `ipfs://` prefix, you need to use the following image reference to point to an image on IPFS.

```
localhost:5050/ipfs/<CID>
```

Here, `CID` is the IPFS CID of the image.

:information_source: In the futural version of nerdctl and BuildKit, `ipfs://` prefix should be supported in Dockerfile.

Using this image reference, you can build an image on IPFS.

```dockerfile
FROM localhost:5050/ipfs/bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze
RUN echo hello > /hello
```

Make sure that `nerdctl ipfs registry serve` is running.
This allows `nerdctl build` to pull images from IPFS.

```
$ nerdctl ipfs registry serve &
```

Then you can build this Dockerfile using `nerdctl build`.

```console
> nerdctl build -t hello .
[+] Building 5.3s (6/6) FINISHED
 => [internal] load build definition from Dockerfile                                                                                              0.0s
 => => transferring dockerfile: 146B                                                                                                              0.0s
 => [internal] load .dockerignore                                                                                                                 0.0s
 => => transferring context: 2B                                                                                                                   0.0s
 => [internal] load metadata for localhost:5050/ipfs/bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze:latest                           0.1s
 => [1/2] FROM localhost:5050/ipfs/bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze@sha256:28bfa1fc6d491d3bee91bab451cab29c747e72917e  3.8s
 => => resolve localhost:5050/ipfs/bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze@sha256:28bfa1fc6d491d3bee91bab451cab29c747e72917e  0.0s
 => => sha256:7b1a6ab2e44dbac178598dabe7cff59bd67233dba0b27e4fbd1f9d4b3c877a54 28.57MB / 28.57MB                                                  2.1s
 => => extracting sha256:7b1a6ab2e44dbac178598dabe7cff59bd67233dba0b27e4fbd1f9d4b3c877a54                                                         1.7s
 => [2/2] RUN echo hello > /hello                                                                                                                 0.6s
 => exporting to oci image format                                                                                                                 0.6s
 => => exporting layers                                                                                                                           0.1s
 => => exporting manifest sha256:b96d490d134221ab121af91a42b13195dd8c5bf941012d7bfe07eabcf5259eda                                                 0.0s
 => => exporting config sha256:bd706574eab19009585b98826b06e63cf6eacf8d7193504dae75caa760332ca2                                                   0.0s
 => => sending tarball                                                                                                                            0.5s
unpacking docker.io/library/hello:latest (sha256:b96d490d134221ab121af91a42b13195dd8c5bf941012d7bfe07eabcf5259eda)...done
> nerdctl run --rm -it hello cat /hello
hello
```

> NOTE: `--ipfs` flag has been removed since v1.2.0. You need to launch the localhost registry by yourself using `nerdctl ipfs registry serve`.

#### Details about `localhost:5050/ipfs/<CID>` and `nerdctl ipfs registry`

As of now, BuildKit doesn't support `ipfs://` prefix so nerdctl achieves builds on IPFS by having a read-only local registry backed by IPFS.
This registry converts registry API requests to IPFS operations.
So IPFS-agnostic tools can pull images from IPFS via this registry.

This registry is provided as a subcommand `nerdctl ipfs registry`.
This command starts the registry backed by the IPFS repo of the current `$IPFS_PATH`
By default, nerdctl exposes the registry at `localhost:5050` (configurable via flags).

<details>
<summary>Creating systemd unit file for `nerdctl ipfs registry`</summary>

Optionally you can create systemd unit file of `nerdctl ipfs registry serve`.
An example systemd unit file for `nerdctl ipfs registry serve` can be the following.
`nerdctl ipfs registry serve` is aware of environemnt variables for configuring the behaviour (e.g. listening port) so you can use `EnvironmentFile` for configuring it.

```
[Unit]
Description=nerdctl ipfs registry serve

[Service]
EnvironmentFile-=/run/nerdctl-ipfs-registry-serve/env
ExecStart=nerdctl ipfs registry serve

[Install]
WantedBy=default.target
```

</details>

The following example starts the registry on `localhost:5555` instead of `localhost:5050`.

```
nerdctl ipfs registry serve --listen-registry=localhost:5555
```

> NOTE: You'll also need to restart the registry when you change `$IPFS_PATH` to use.

> NOTE: `nerdctl ipfs registry [up|down]` has been removed since v1.2.0. You need to launch the localhost registry using `nerdctl ipfs registry serve` instead.

### Compose on IPFS

`nerdctl compose` supports same image name syntax to pull images from IPFS.

```yaml
version: "3.8"
services:
  ubuntu:
    image: ipfs://bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze
    command: echo hello
```

When you build images using base images on IPFS, you can use `localhost:5050/ipfs/<CID>` image reference in Dockerfile as mentioned above.

```
nerdctl compose up --build
```

```
nerdctl compose build
```

> NOTE: `--ipfs` flag has been removed since v1.2.0. You need to launch the localhost registry by yourself using `nerdctl ipfs registry serve`.

### Encryption

You can distribute [encrypted images](./ocicrypt.md) on IPFS using OCIcrypt.
Please see [`/docs/ocicrypt.md`](./ocicrypt.md) for details about how to encrypt and decrypt an image.

Same as normal images, the encrypted image can be pushed to IPFS using `ipfs://` prefix.

```console
> nerdctl image encrypt --recipient=jwe:mypubkey.pem ubuntu:20.04 ubuntu:20.04-encrypted
sha256:a5c57411f3d11bb058b584934def0710c6c5b5a4a2d7e9b78f5480ecfc450740
> nerdctl push ipfs://ubuntu:20.04-encrypted
INFO[0000] pushing image "ubuntu:20.04-encrypted" to IPFS
INFO[0000] ensuring image contents
bafkreifajsysbvhtgd7fdgrfesszexdq6v5zbj5y2jnjfwxdjyqws2s3s4
```

You can pull the encrypted image from IPFS using `ipfs://` prefix and can decrypt it in the same way as described in [`/docs/ocicrypt.md`](./ocicrypt.md).

```console
> nerdctl pull --unpack=false ipfs://bafkreifajsysbvhtgd7fdgrfesszexdq6v5zbj5y2jnjfwxdjyqws2s3s4
bafkreifajsysbvhtgd7fdgrfesszexdq6v5zbj5y2jnjfwxdjyqws2s3s4:                      resolved       |++++++++++++++++++++++++++++++++++++++|
index-sha256:73334fee83139d1d8dbf488b28ad100767c38428b2a62504c758905c475c1d6c:    done           |++++++++++++++++++++++++++++++++++++++|
manifest-sha256:8855ae825902045ea2b27940634673ba410b61885f91b9f038f6b3303f48727c: done           |++++++++++++++++++++++++++++++++++++++|
config-sha256:ba6acccedd2923aee4c2acc6a23780b14ed4b8a5fa4e14e252a23b846df9b6c1:   done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:e74a9a7749e808e4ad1e90d5a81ce3146ce270de0fbdf22429cd465df8f10a13:    done           |++++++++++++++++++++++++++++++++++++++|
elapsed: 0.3 s                                                                    total:  22.0 M (73.2 MiB/s)
> nerdctl image decrypt --key=mykey.pem ipfs://bafkreifajsysbvhtgd7fdgrfesszexdq6v5zbj5y2jnjfwxdjyqws2s3s4 ubuntu:20.04-decrypted
sha256:b0ccaddb7e7e4e702420de126468eab263eb0f3c25abf0b957ce8adcd1e82105
> nerdctl run --rm -it ubuntu:20.04-decrypted echo hello
hello
```

## Running containers on IPFS with eStargz-based lazy pulling

nerdctl supports running eStargz images on IPFS with lazy pulling using Stargz Snapshotter.

In this configuration, Stargz Snapshotter mounts the eStargz image from IPFS to the container's rootfs using FUSE with lazy pulling support.
Thus the container can startup without waiting for the entire image contents to be locally available.
You can see faster container cold-start.

To use this feature, you need to enable Stargz Snapshotter following [`/docs/stargz.md`](./stargz.md).
You also need to add the following configuration to `config.toml` of Stargz Snapshotter (typically located at `/etc/containerd-stargz-grpc/config.toml`).

```toml
ipfs = true
```

You can push an arbitrary image to IPFS with converting it to eStargz using `--estargz` option.

```
nerdctl push --estargz ipfs://fedora:36
```

You can pull and run that eStargz image with lazy pulling.

```
nerdctl run --rm -it ipfs://bafkreibp2ncujcia663uum25ustwvmyoguxqyzjnxnlhebhsgk2zowscye echo hello
```

- See [the doc in stargz-snapshotter project](https://github.com/containerd/stargz-snapshotter/blob/v0.10.0/docs/ipfs.md) for details about lazy pulling on IPFS.
- See [`/docs/stargz.md`](./stargz.md) for details about the configuration of nerdctl for Stargz Snapshotter.
