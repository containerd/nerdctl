# Container Image Sign and Verify with cosign tool

[cosign](https://github.com/sigstore/cosign) is tool that allows you to sign and verify container images with the
public/private key pairs or without them by providing
a [Keyless support](https://github.com/sigstore/cosign/blob/main/KEYLESS.md).

Keyless uses ephemeral keys and certificates, which are signed automatically by
the [fulcio](https://github.com/sigstore/fulcio) root CA. Signatures are stored in
the [rekor](https://github.com/sigstore/rekor) transparency log, which automatically provides an attestation as to when
the signature was created.

You can enable container signing and verifying features with `push` and `pull` commands of `nerdctl` by using `cosign`
under the hood with make use of flags `--sign` while pushing the container image, and `--verify` while pulling the
container image.

> * Ensure cosign executable in your `$PATH`.
> * You can install cosign by following this page: https://docs.sigstore.dev/cosign/installation

Prepare your environment:

```shell
# Create a sample Dockerfile
$ cat <<EOF | tee Dockerfile.dummy
FROM alpine:latest
CMD [ "echo", "Hello World" ]
EOF
```

> Please do not forget, we won't be validating the base images, which is `alpine:latest` in this case, of the container image that was built on,
> we'll only verify the container image itself once we sign it.

```shell

# Build the image
$ nerdctl build -t devopps/hello-world -f Dockerfile.dummy .

# Generate a key-pair: cosign.key and cosign.pub
$ cosign generate-key-pair

# Export your COSIGN_PASSWORD to prevent CLI prompting
$ export COSIGN_PASSWORD=$COSIGN_PASSWORD
```

Sign the container image while pushing:

```
# Sign the image with Keyless mode
$ nerdctl push --sign=cosign devopps/hello-world

# Sign the image and store the signature in the registry
$ nerdctl push --sign=cosign --cosign-key cosign.key devopps/hello-world
```

Verify the container image while pulling:

> REMINDER: Image won't be pulled if there are no matching signatures in case you passed `--verify` flag.

```shell
# Verify the image with Keyless mode
$ nerdctl pull --verify=cosign devopps/hello-world
INFO[0004] cosign:
INFO[0004] cosign: [{"critical":{"identity":...}]
docker.io/devopps/nginx-new:latest:                                               resolved       |++++++++++++++++++++++++++++++++++++++|
manifest-sha256:0910d404e58dd320c3c0c7ea31bf5fbfe7544b26905c5eccaf87c3af7bcf9b88: done           |++++++++++++++++++++++++++++++++++++++|
config-sha256:1de1c4fb5122ac8650e349e018fba189c51300cf8800d619e92e595d6ddda40e:   done           |++++++++++++++++++++++++++++++++++++++|
elapsed: 1.4 s                                                                    total:  1.3 Ki (928.0 B/s)

# You can not verify the image if it is not signed
$ nerdctl pull --verify=cosign --cosign-key cosign.pub devopps/hello-world-bad
INFO[0003] cosign: Error: no matching signatures:
INFO[0003] cosign: failed to verify signature
INFO[0003] cosign: main.go:46: error during command execution: no matching signatures:
INFO[0003] cosign: failed to verify signature
```
