# Health Check Support in nerdctl [WIP]

`nerdctl` supports Docker-compatible health checks for containers, allowing users to monitor container health via a user-defined command.

Health checks can be configured in multiple ways:

1. At container creation time using nerdctl run or nerdctl create with --health-* flags
2. At image build time using HEALTHCHECK in a Dockerfile
3. In docker-compose.yaml files, if using nerdctl compose

When a container is created, nerdctl determines the health check configuration based on the following priority:

1. **CLI flags** take highest precedence (e.g., `--health-cmd`, etc.)
2. If no CLI flags are set, nerdctl will use any health check defined in the image.
3. If neither is present, no health check will be configured

### Disabling Health Checks

You can disable health checks using the following flag during container create/run:

```bash
--no-healthcheck
```
---

### Example

```bash
nerdctl run --name web --health-cmd="curl -f http://localhost || exit 1" --health-interval=30s --health-timeout=5s --health-retries=3 nginx
```
