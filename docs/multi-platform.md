# Multi-platform

| :zap: Requirement | nerdctl >= 0.13 |
|-------------------|-----------------|

nerdctl can execute non-native container images using QEMU.
e.g., ARM on Intel, and vice versa.

## Preparation: Register QEMU to `/proc/sys/fs/binfmt_misc`

```console
$ sudo systemctl start containerd

$ sudo nerdctl run --privileged --rm tonistiigi/binfmt:master --install all

$ ls -1 /proc/sys/fs/binfmt_misc/qemu*
/proc/sys/fs/binfmt_misc/qemu-aarch64
/proc/sys/fs/binfmt_misc/qemu-arm
/proc/sys/fs/binfmt_misc/qemu-loongarch64
/proc/sys/fs/binfmt_misc/qemu-mips64
/proc/sys/fs/binfmt_misc/qemu-mips64el
/proc/sys/fs/binfmt_misc/qemu-ppc64le
/proc/sys/fs/binfmt_misc/qemu-riscv64
/proc/sys/fs/binfmt_misc/qemu-s390x
```

The `tonistiigi/binfmt` container must be executed with `--privileged`, and with rootful mode (`sudo`).

This container is not a daemon, and exits immediately after registering QEMU to `/proc/sys/fs/binfmt_misc`.
Run `ls -1 /proc/sys/fs/binfmt_misc/qemu*` to confirm registration.

See also https://github.com/tonistiigi/binfmt

## Usage
### Pull & Run

```console
$ nerdctl pull --platform=arm64,s390x alpine

$ nerdctl run --rm --platform=arm64 alpine uname -a
Linux e6227935cf12 5.13.0-19-generic #19-Ubuntu SMP Thu Oct 7 21:58:00 UTC 2021 aarch64 Linux

$ nerdctl run --rm --platform=s390x alpine uname -a
Linux b39da08fbdbf 5.13.0-19-generic #19-Ubuntu SMP Thu Oct 7 21:58:00 UTC 2021 s390x Linux
```

### Build & Push
```console
$ nerdctl build --platform=amd64,arm64 --output type=image,name=example.com/foo:latest,push=true .
```

Or

```console
$ nerdctl build --platform=amd64,arm64 -t example.com/foo:latest .
$ nerdctl push --all-platforms example.com/foo:latest
```

### Compose
See [`../examples/compose-multi-platform`](../examples/compose-multi-platform)

## macOS + Lima

As of 2025-03-01, qemu seems to be broken in most Apple-silicon setups.
This might be due to qemu handling of host vs. guest page sizes
(unconfirmed, see https://github.com/containerd/nerdctl/issues/3948 for more information).

It should also be noted that Linux 6.11 introduced a change to the VDSO (on ARM)
that does break Rosetta.

The take-away here is that presumably your only shot at running non-native binaries
on Apple-silicon is to use an older kernel for your guest (<6.11), typically as shipped by Debian stable,
and also to use VZ+Rosetta and not qemu (eg: `limactl create --vm-type=vz --rosetta`).