# Container Image Sign and Verify with cosign tool

| :zap: Requirement | nerdctl >= 0.15 |
|-------------------|-----------------|

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

## Cosign in Compose

> Cosign support in Compose is also experimental and implemented based on Compose's [extension](https://github.com/compose-spec/compose-spec/blob/master/spec.md#extension) capibility.

cosign is supported in `nerdctl compose up|run|push|pull`. You can use cosign in Compose by adding the following fields in your compose yaml. These fields are _per service_, and you can enable only `verify` or only `sign` (or both).

```yaml
# only put cosign related fields under the service you want to sign/verify.
services:
  svc0:
    build: .
    image: ${REGISTRY}/svc0_image # replace with your registry
    # `x-nerdctl-verify` and `x-nerdctl-cosign-public-key` are for verify
    # required for `nerdctl compose up|run|pull`
    x-nerdctl-verify: cosign
    x-nerdctl-cosign-public-key: /path/to/cosign.pub
    # `x-nerdctl-sign` and `x-nerdctl-cosign-private-key` are for sign
    # required for `nerdctl compose push`
    x-nerdctl-sign: cosign
    x-nerdctl-cosign-private-key: /path/to/cosign.key
    ports:
    - 8080:80
  svc1:
    build: .
    image: ${REGISTRY}/svc1_image # replace with your registry
    ports:
    - 8081:80
```

Following the cosign tutorial above, first set up environment and prepare cosign key pair:

```shell
# Generate a key-pair: cosign.key and cosign.pub
$ cosign generate-key-pair

# Export your COSIGN_PASSWORD to prevent CLI prompting
$ export COSIGN_PASSWORD=$COSIGN_PASSWORD
```

We'll use the following `Dockerfile` and `docker-compose.yaml`:

```shell
$ cat Dockerfile
FROM nginx:1.19-alpine
RUN uname -m > /usr/share/nginx/html/index.html

$ cat docker-compose.yml
services:
  svc0:
    build: .
    image: ${REGISTRY}/svc1_image # replace with your registry
    x-nerdctl-verify: cosign
    x-nerdctl-cosign-public-key: ./cosign.pub
    x-nerdctl-sign: cosign
    x-nerdctl-cosign-private-key: ./cosign.key
    ports:
    - 8080:80
  svc1:
    build: .
    image: ${REGISTRY}/svc1_image # replace with your registry
    ports:
    - 8081:80
```

> The `env "COSIGN_PASSWORD="$COSIGN_PASSWORD""` part in the below commands is a walkaround to use rootful nerdctl and make the env variable visible to root (in sudo). You don't need this part if (1) you're using rootless, or (2) your `COSIGN_PASSWORD` is visible in root.

First let's `build` and `push` the two services:

```shell
$ sudo nerdctl compose build
INFO[0000] Building image xxxxx/svc0_image
...
INFO[0000] Building image xxxxx/svc1_image
[+] Building 0.2s (6/6) FINISHED

$ sudo env "COSIGN_PASSWORD="$COSIGN_PASSWORD"" nerdctl compose --experimental=true push
INFO[0000] Pushing image xxxxx/svc1_image
...
INFO[0000] Pushing image xxxxx/svc0_image
INFO[0000] pushing as a reduced-platform image (application/vnd.docker.distribution.manifest.v2+json, sha256:4329abc3143b1545835de17e1302c8313a9417798b836022f4c8c8dc8b10a3e9)
INFO[0000] cosign: WARNING: Image reference xxxxx/svc0_image uses a tag, not a digest, to identify the image to sign.
INFO[0000] cosign:
INFO[0000] cosign: This can lead you to sign a different image than the intended one. Please use a
INFO[0000] cosign: digest (example.com/ubuntu@sha256:abc123...) rather than tag
INFO[0000] cosign: (example.com/ubuntu:latest) for the input to cosign. The ability to refer to
INFO[0000] cosign: images by tag will be removed in a future release.
INFO[0000] cosign: Pushing signature to: xxxxx/svc0_image
```

Then we can `pull` and `up` services (`run` is similar to up):

```shell
# ensure built images are removed and pull is performed.
$ sudo nerdctl compose down
$ sudo env "COSIGN_PASSWORD="$COSIGN_PASSWORD"" nerdctl compose --experimental=true pull
$ sudo env "COSIGN_PASSWORD="$COSIGN_PASSWORD"" nerdctl compose --experimental=true up
$ sudo env "COSIGN_PASSWORD="$COSIGN_PASSWORD"" nerdctl compose --experimental=true run svc0 -- echo "hello"
# clean up compose resources.
$ sudo nerdctl compose down
```

Check your logs to confirm that svc0 is verified by cosign (have cosign logs) and svc1 is not. You can also change the public key in `docker-compose.yaml` to a random value to see verify failure will stop the container being `pull|up|run`.