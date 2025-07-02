# Building nerdctl

To build nerdctl, use `make`:

```bash
make
sudo make install
```

Alternatively, nerdctl can be also built with `go build ./cmd/nerdctl`.
However, this is not recommended as it does not populate the version string (`nerdctl -v`).

## Customization

To specify build tags, set the `BUILDTAGS` variable as follows:

```bash
BUILDTAGS=no_ipfs make
```

Multiple build tags can be combined by separating them with spaces:

```bash
BUILDTAGS="no_ipfs no_stargz no_nydus" make
```

The following build tags are supported:
* `no_ipfs` (since v2.1.3): Disable IPFS
* `no_stargz` (since v2.1.3): Disable stargz snapshotter support
* `no_nydus` (since v2.1.3): Disable nydus snapshotter support
* `no_overlaybd` (since v2.1.3): Disable overlaybd snapshotter support
