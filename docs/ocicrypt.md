# OCIcrypt


See https://github.com/containerd/imgcrypt to learn further information.

## Encryption

See https://github.com/containerd/imgcrypt

## Decryption

### Configuration
Add the following configuration to `/etc/containerd/config.toml` (for rootless `~/.config/containerd/config.toml`):

```toml
version = 2

[stream_processors]
  [stream_processors."io.containerd.ocicrypt.decoder.v1.tar.gzip"]
    accepts = ["application/vnd.oci.image.layer.v1.tar+gzip+encrypted"]
    returns = "application/vnd.oci.image.layer.v1.tar+gzip"
    path = "ctd-decoder"
    args = ["--decryption-keys-path", "/etc/containerd/ocicrypt/keys"]
  [stream_processors."io.containerd.ocicrypt.decoder.v1.tar"]
    accepts = ["application/vnd.oci.image.layer.v1.tar+encrypted"]
    returns = "application/vnd.oci.image.layer.v1.tar"
    path = "ctd-decoder"
    args = ["--decryption-keys-path", "/etc/containerd/ocicrypt/keys"]

# NOTE: On rootless, ~/.config/containerd is mounted as /etc/containerd in the namespace.
```

Future version of containerd may have this configuration by default: https://github.com/containerd/containerd/pull/5135

Then, put the private key files to `/etc/containerd/ocicrypt/keys` (for rootless `~/.config/containerd/ocicrypt/keys`).

### nerdctl run

No flag is needed for running encrypted images with `nerdctl run`.

Just run `nerdctl run example.com/encrypted-image`.
