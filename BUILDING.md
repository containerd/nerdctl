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

The following build tags are supported:
* `no_ipfs` (since v2.1.3): Disable IPFS
* `no_stargz` (since v2.1.3): Disable stargz snapshotter support
