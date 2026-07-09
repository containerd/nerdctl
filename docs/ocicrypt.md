# OCIcrypt

| :zap: Requirement | nerdctl >= 0.7 |
|-------------------|----------------|

nerdctl supports encryption and decryption using [OCIcrypt](https://github.com/containers/ocicrypt)
(aka [imgcrypt](https://github.com/containerd/imgcrypt) for containerd).

## JWE mode

### Encryption

Use `openssl` to create a private key (`mykey.pem`) and the corresponding public key (`mypubkey.pem`):
```bash
openssl genrsa -out mykey.pem
openssl rsa -in mykey.pem -pubout -out mypubkey.pem
```

Use `nerdctl image encrypt` to create an encrypted image:
```bash
nerdctl image encrypt --recipient=jwe:mypubkey.pem --platform=linux/amd64,linux/arm64 foo example.com/foo:encrypted
nerdctl push example.com/foo:encrypted
```

:warning: CAUTION: This command only encrypts image layers, but does NOT encrypt [container configuration such as `Env` and `Cmd`](https://github.com/opencontainers/image-spec/blob/v1.0.1/config.md#example).
To see non-encrypted information, run `nerdctl image inspect --mode=native --platform=PLATFORM example.com/foo:encrypted` .

### Decryption

#### Configuration
Put the private key files to `/etc/containerd/ocicrypt/keys` (for rootless `~/.config/containerd/ocicrypt/keys`).

<details>
<summary>Extra step for containerd 1.4 and older</summary>

<p>

containerd 1.4 and older requires adding the following configuration to `/etc/containerd/config.toml`
(for rootless `~/.config/containerd/config.toml`):

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

</p>

</details>

#### Running nerdctl

No flag is needed for running encrypted images with `nerdctl run`, as long as the private key is stored
in `/etc/containerd/ocicrypt/keys` (for rootless `~/.config/containerd/ocicrypt/keys`).

Just run `nerdctl run example.com/encrypted-image`.

To decrypt an image without running a container, use `nerdctl image decrypt` command:
```bash
nerdctl pull --unpack=false example.com/foo:encrypted
nerdctl image decrypt --key=mykey.pem example.com/foo:encrypted foo:decrypted
```

## PGP (GPG) mode
(Undocumented yet)

## PKCS7 mode
(Undocumented yet)

## PKCS11 mode
(Undocumented yet)

## More information
- https://github.com/containerd/imgcrypt (High-level library for containerd, using `containers/ocicrypt`)
- https://github.com/containers/ocicrypt (Low-level library, used by `containerd/imgcrypt`)
- https://github.com/opencontainers/image-spec/pull/775 (Proposal for OCI Image Spec)
- https://github.com/containerd/containerd/blob/main/docs/cri/decryption.md (configuration guide)
  - The `plugins."io.containerd.grpc.v1.cri"` section does not apply to nerdctl, as nerdctl does not use CRI
