# Experimental features of nerdctl

The following features are experimental and subject to change:

- [Windows containers](https://github.com/containerd/nerdctl/issues/28)
- [FreeBSD containers](./freebsd.md)
- Importing an external eStargz record JSON file with `nerdctl image convert --estargz-record-in=FILE` .
  eStargz itself is out of experimental.
- [Image Distribution on IPFS](./ipfs.md)
- [Image Sign and Verify (cosign)](./cosign.md)
- [Rootless container networking acceleration with bypass4netns](./rootless.md#bypass4netns)
- [Interactive debugging of Dockerfile](./builder-debug.md)
