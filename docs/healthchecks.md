# Health Check Support in nerdctl

`nerdctl` supports Docker-compatible health checks for containers, allowing users to monitor container health via a user-defined command.

Currently, health checks can be triggered manually using the nerdctl container healthcheck command. Automatic orchestration (e.g., periodic checks) will be added in a future update.

Health checks can be configured in multiple ways:

1. At container creation time using nerdctl run or nerdctl create with `--health-*` flags
2. At image build time using HEALTHCHECK in a Dockerfile
3. In docker-compose.yaml files, if using nerdctl compose

When a container is created, nerdctl determines the health check configuration based on the following priority:

1. **CLI flags** take highest precedence (e.g., `--health-cmd`, etc.)
2. If no CLI flags are set, nerdctl will use any health check defined in the image.
3. If neither is present, no health check will be configured

Example:

```bash
nerdctl run --name web --health-cmd="curl -f http://localhost || exit 1" --health-interval=30s --health-timeout=5s --health-retries=3 nginx
```

### Disabling Health Checks

You can disable health checks using the following flag during container create/run:

```bash
--no-healthcheck
```

### Running Health Checks Manually

nerdctl provides a container healthcheck command that can be manually triggered by the user. This command runs the
configured health check inside the container and reports the result. It serves as the entry point for executing
health checks, especially in scenarios where external scheduling is used.

Example:

```
nerdctl container healthcheck <container-id>
```

### Future Work (WIP)

Since nerdctl is daemonless and does not have a persistent background process, we rely on systemd(or external schedulers)
to invoke nerdctl container healthcheck at configured intervals. This allows periodic health checks for containers in a
systemd-based environment. We are actively working on automating health checks, where we will listen to container lifecycle
events and generate appropriate systemd service and timer units. This will enable nerdctl to support automated,
Docker-compatible health checks by leveraging systemd for scheduling and lifecycle integration.