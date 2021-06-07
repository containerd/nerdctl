# nerdctl compose

## Usage

The `nerdctl compose` CLI is designed to be compatible with `docker-compose`.

```console
$ nerdctl compose up -d
$ nerdctl compose down
```

See the Command Reference in [`../README.md`](../README.md).

## Spec conformance

`nerdctl compose` implements [The Compose Specification](https://github.com/compose-spec/compose-spec),
which was derived from [Docker Compose file version 3 specification](https://docs.docker.com/compose/compose-file/compose-file-v3/).

### Unimplemented YAML fields
- Fields that correspond to unimplemented `docker run` flags, e.g., `services.<SERVICE>.links` (corresponds to `docker run --link`)
- Fields that correspond to unimplemented `docker build` flags, e.g., `services.<SERVICE>.build.labels` (corresponds to `docker build --label`)
- `services.<SERVICE>.credential_spec`
- `services.<SERVICE>.deploy.update_config`
- `services.<SERVICE>.deploy.rollback_config`
- `services.<SERVICE>.deploy.resources.reservations`
- `services.<SERVICE>.deploy.placement`
- `services.<SERVICE>.deploy.endpoint_mode`
- `services.<SERVICE>.healthcheck`
- `services.<SERVICE>.profiles`
- `services.<SERVICE>.stop_grace_period`
- `services.<SERVICE>.stop_signal`
- `configs.<CONFIG>.external`
- `secrets.<SECRET>.external`

### Incompatibility
#### `services.<SERVICE>.build.context`
- The value must be a local directory path, not a URL.

#### `services.<SERVICE>.entrypoint`
- Multiple entrypoint strings cannot be specified.

#### `services.<SERVICE>.secrets`, `services.<SERVICE>.configs`
- `uid`, `gid`: Cannot be specified. The default value is not propagated from `USER` instruction of Dockerfile.
  The file owner corresponds to the original file on the host.
- `mode`: Cannot be specified. The file is mounted as read-only, with permission bits that correspond to the original file on the host.
