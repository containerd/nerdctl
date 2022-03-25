# Setting up `nerdctl build` with BuildKit

`nerdctl build` (and `nerdctl compose build`) relies on [BuildKit](https://github.com/moby/buildkit).
To use it, you need to set up BuildKit.

BuildKit has 2 types of backends.

- **containerd worker**: BuildKit relies on containerd to manage containers and images, etc. containerd needs to be up-and-running on the host.
- **OCI worker**: BuildKit manages containers and images, etc. containerd isn't needed. This worker relies on runc for container execution.

You need to set up BuildKit with either of the above workers.

Note that OCI worker cannot access base images (`FROM` images in Dockerfiles) managed by containerd.
Thus you cannot let `nerdctl build` use containerd-managed images as the base image.
They include images previously built using `nerdctl build`.

For example, the following build `bar` fails with OCI worker because it tries to use the previously built and containerd-managed image `foo`.

```console
$ mkdir -p /tmp/ctx && cat <<EOF > /tmp/ctx/Dockerfile
FROM ghcr.io/stargz-containers/ubuntu:20.04-org
RUN echo hello
EOF
$ nerdctl build -t foo /tmp/ctx
$ cat <<EOF > /tmp/ctx/Dockerfile
FROM foo
RUN echo bar
EOF
$ nerdctl build -t bar /tmp/ctx
```

This limitation can be avoided using containerd worker as mentioned later.

## Setting up BuildKit with containerd worker

### Rootless

| :zap: Requirement | nerdctl >= 0.18, BuildKit >= 0.10 |
|-------------------|-----------------------------------|

```
$ CONTAINERD_NAMESPACE=default containerd-rootless-setuptool.sh install-buildkit-containerd
```

`containerd-rootless-setuptool.sh` is aware of `CONTAINERD_NAMESPACE` and `CONTAINERD_SNAPSHOTTER` envvars.
It installs buildkitd to the specified containerd namespace.
This allows BuildKit using containerd-managed images in that namespace as the base image.
Note that BuildKit can't use images in other namespaces as of now.

If `CONTAINERD_NAMESPACE` envvar is not specified, this script configures buildkitd to use "buildkit" namespace (not "default" namespace).

You can install an additional buildkitd process in a different namespace by executing this script with specifying the namespace with `CONTAINERD_NAMESPACE`.

BuildKit will expose the socket at `$XDG_RUNTIME_DIR/buildkit-$CONTAINERD_NAMESPACE/buildkitd.sock` if `CONTAINERD_NAMESPACE` is specified.
If `CONTAINERD_NAMESPACE` is not specified, that location will be `$XDG_RUNTIME_DIR/buildkit/buildkitd.sock`.

### Rootful

```
$ sudo systemctl enable --now buildkit
```

Then add the following configuration to `/etc/buildkit/buildkitd.toml` to enable containerd worker.

```toml
[worker.oci]
  enabled = false

[worker.containerd]
  enabled = true
  # namespace should be "k8s.io" for Kubernetes (including Rancher Desktop)
  namespace = "default"
```

## Setting up BuildKit with OCI worker

### Rootless

```
$ containerd-rootless-setuptool.sh install-buildkit
```

As mentioned in the above, BuildKit with this configuration cannot use images managed by containerd.
They include images previously built with `nerdctl build`.

BuildKit will expose the socket at `$XDG_RUNTIME_DIR/buildkit/buildkitd.sock`.

### rootful

```
$ sudo systemctl enable --now buildkit
```

## Which BuildKit socket will nerdctl use?

You can specify BuildKit address for `nerdctl build` using `--buildkit-host` flag or `BUILDKIT_HOST` envvar.
When BuildKit address isn't specified, nerdctl tries some default BuildKit addresses the following order and uses the first available one.

- `<runtime directory>/buildkit-<current namespace>/buildkitd.sock`
- `<runtime directory>/buildkit-default/buildkitd.sock`
- `<runtime directory>/buildkit/buildkitd.sock`

For example, if you run rootless nerdctl with `test` containerd namespace, it tries to use `$XDG_RUNTIME_DIR/buildkit-test/buildkitd.sock` by default then try to fall back to `$XDG_RUNTIME_DIR/buildkit-default/buildkitd.sock` and `$XDG_RUNTIME_DIR/buildkit/buildkitd.sock`
