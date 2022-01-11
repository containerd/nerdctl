# registry authentication

nerdctl uses `${DOCKER_CONFIG}/config.json` for the authentication with image registries.

`$DOCKER_CONFIG` defaults to `$HOME/.docker`.

## Using insecure registry

If you face `http: server gave HTTP response to HTTPS client` and you cannot configure TLS for the registry, try `--insecure-registry` flag:

e.g.,
```console
$ nerdctl --insecure-registry run --rm 192.168.12.34:5000/foo
```

## Specifying certificates


| :zap: Requirement | nerdctl >= 0.16 |
|-------------------|-----------------|


Create `~/.config/containerd/certs.d/<HOST:PORT>/hosts.toml` (or `/etc/containerd/certs.d/...` for rootful) to specify `ca` certificates.

```toml
# An example of ~/.config/containerd/certs.d/192.168.12.34:5000/hosts.toml
# (The path is "/etc/containerd/certs.d/192.168.12.34:5000/hosts.toml" for rootful)

server = "https://192.168.12.34:5000"
[host."https://192.168.12.34:5000"]
  ca = "/path/to/ca.crt"
```

See https://github.com/containerd/containerd/blob/main/docs/hosts.md for the syntax of `hosts.toml` .

Docker-style directories are also supported.
The path is `~/.config/docker/certs.d` for rootless, `/etc/docker/certs.d` for rootful.

## Accessing 127.0.0.1 from rootless nerdctl

Currently, rootless nerdctl cannot pull images from 127.0.0.1, because
the pull operation occurs in RootlessKit's network namespace.

See https://github.com/containerd/nerdctl/issues/86 for the discussion about workarounds.
