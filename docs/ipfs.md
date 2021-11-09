# Distribute Container Images on IPFS (Experimental)

You can distribute container images without registries, using IPFS.

## Prerequisites

To use this feature, make sure `ipfs daemon` is running on your host.
For example, you can run an IPFS daemon using the following command.

```
ipfs daemon
```

In rootless mode, you need to install ipfs daemon using `containerd-rootless-setuptool.sh`.

```
containerd-rootless-setuptool.sh -- install-ipfs --init
```

:information_source: If you don't want IPFS to communicate with nodes on the internet, you can run IPFS daemon in offline mode using `--offline` flag or you can create a private IPFS network as described [here](https://github.com/containerd/stargz-snapshotter/blob/main/docs/ipfs.md#appendix-1-creating-ipfs-private-network).

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

:information_source: Note that though the IPFS-enabled image is OCI compatible, some runtimes including [containerd](https://github.com/containerd/containerd/pull/6221) and [podman](https://github.com/containers/image/pull/1403) had bugs and failed to pull that image. Containerd has already fixed this.

### compose on IPFS

`nerdctl compose` supports same image name syntax to pull images from IPFS.

```yaml
version: "3.8"
services:
  ubuntu:
    image: ipfs://bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze
    command: echo hello
```

### encryption

You can distribute [encrypted images](./ocicrypt.md) on IPFS using OCIcrypt.
Please see [`/docs/ocycrypt.md`](./ocicrypt.md) for details about how to ecrypt and decrypt an image.

Same as normal images, the encrypted image can be pushed to IPFS using `ipfs://` prefix.

```console
> nerdctl image encrypt --recipient=jwe:mypubkey.pem ubuntu:20.04 ubuntu:20.04-encrypted
sha256:a5c57411f3d11bb058b584934def0710c6c5b5a4a2d7e9b78f5480ecfc450740
> nerdctl push ipfs://ubuntu:20.04-encrypted
INFO[0000] pushing image "ubuntu:20.04-encrypted" to IPFS
INFO[0000] ensuring image contents
bafkreifajsysbvhtgd7fdgrfesszexdq6v5zbj5y2jnjfwxdjyqws2s3s4
```

You can pull the encrypted image from IPFS using `ipfs://` prefix and can decrypt it in the same way as described in [`/docs/ocycrypt.md`](./ocicrypt.md).

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
You also need to add the following configuration to `config.toml` of Stargz Snapsohtter (typically located at `/etc/containerd-stargz-grpc/config.toml`).

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
