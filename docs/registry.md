# registry authentication

nerdctl uses `${DOCKER_CONFIG}/config.json` for the authentication with image registries.

`$DOCKER_CONFIG` defaults to `$HOME/.docker`.

## Using insecure registry

If you face `http: server gave HTTP response to HTTPS client` and you cannot configure TLS for the registry, try `--insecure-registry` flag:

e.g.,
```console
$ nerdctl --insecure-registry run --rm 192.168.12.34:5000/foo
```

## Accessing 127.0.0.1 from rootless nerdctl

Currently, rootless nerdctl cannot pull images from 127.0.0.1, because
the pull operation occurs in RootlessKit's network namespace.

See https://github.com/containerd/nerdctl/issues/86 for the discussion about workarounds.
