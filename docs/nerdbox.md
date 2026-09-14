# nerdbox (experimental)

| :zap: Requirement | nerdctl >= 2.2, containerd >= 2.2 |
|-------------------|-----------------------------------|


nerdctl supports [nerdbox](https://github.com/containerd/nerdbox) experimentally
for running Linux containers, including non-Linux hosts such as macOS.

## Prerequisites
- [nerdbox](https://github.com/containerd/nerdbox) with its dependencies

- `/var/lib/nerdctl` directory chowned for the current user:
```bash
sudo mkdir -p /var/lib/nerdctl
sudo chown $(whoami):staff /var/lib/nerdctl
```

## Usage

```bash
nerdctl run \
  --rm \
  --snapshotter erofs \
  --runtime io.containerd.nerdbox.v1 \
  --platform=linux/arm64 \
  --net=host \
  --log-driver=none \
  hello-world
```

Lots of CLI flags still do not work.

## FAQ
### How is nerdbox comparable to Lima ?

The following table compares nerdbox and [Lima](https://lima-vm.io/)
on macOS for running Linux containers:

|                       | nerdbox | Lima        |
|-----------------------|---------|-------------|
| #VM : #Container      | 1:1     | 1:N         |
| VM image              | minimal | full Ubuntu |
| containerd running on | host    | inside VM   |
| Low-level runtime     | krun    | runc        |
| Snapshotter           | erofs   | overlayfs   |

See also:
- https://github.com/containerd/nerdbox
- https://lima-vm.io/docs/examples/containers/containerd/
