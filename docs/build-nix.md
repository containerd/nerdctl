# Reproducible image building using Nix

`nerdctl build-nix` (EXPERIMENTAL) allows [reproducible image building](https://en.wikipedia.org/wiki/Reproducible_builds) using Nix.

## Example

See [`../examples/build-nix/nginx`](../examples/build-nix/nginx)

```
nerdctl build-nix
nerdctl run -d -p 8080:80 nginx:nix
```

nerdctl executes Nix in a container, so Nix does NOT need to be installed on the host.

## Pinning packages

Use `niv`, see https://nix.dev/tutorials/towards-reproducibility-pinning-nixpkgs
