# Auditing dockerfile

Because of the nature of GitHub cache, and the time it takes to build the dockerfile for testing, it is desirable
to be able to audit what is going on there.

This document provides a few pointers on how to do that, and some results as of 2025-02-26 (run inside lima, nerdctl main,
on a macbook pro M1).

## Intercept network traffic

### On macOS

Use Charles:
- start SSL proxying
- enable SOCKS proxy
- export the root certificate

### On linux

Left as an exercise to the reader.

### If using lima

- restart your lima instance with `HTTP_PROXY=http://X.Y.Z.W:8888 HTTPS_PROXY=socks5://X.Y.Z.W:8888 limactl start instance` - where XYZW
is the local ip of the Charles proxy (non-localhost)

### On the host where you are running containerd

- copy the root certificate from above into `/usr/local/share/ca-certificates/charles-ssl-proxying-certificate.crt`
- update your host: `sudo update-ca-certificates`
- now copy the root certificate again to your current nerdctl clone

### Hack the dockerfile to insert our certificate

Add the following stages in the dockerfile:
```dockerfile
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS hack-build-base-debian
RUN apt-get update -qq; apt-get -qq install ca-certificates
COPY charles-ssl-proxying-certificate.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS hack-build-base
RUN apk add --no-cache ca-certificates
COPY charles-ssl-proxying-certificate.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates

FROM ubuntu:${UBUNTU_VERSION} AS hack-base
RUN apt-get update -qq; apt-get -qq install ca-certificates
COPY charles-ssl-proxying-certificate.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates
```

Then replace any later "FROM" with our modified bases:
```
golang:${GO_VERSION}-bookworm => hack-build-base-debian
golang:${GO_VERSION}-alpine => hack-build-base
ubuntu:${UBUNTU_VERSION} => hack-base
```

## Mimicking what the CI is doing

A quick helper:

```bash
run(){
  local no_cache="${1:-}"
  local platform="${2:-arm64}"
  local dockerfile="${3:-Dockerfile}"
  local target="${4:-test-integration}"

  local cache_shard="$CONTAINERD_VERSION"-"$platform"
  local shard="$cache_shard"-"$target"-"$UBUNTU_VERSION"-"$no_cache"-"$dockerfile"

  local cache_location=$HOME/bk-cache-"$cache_shard"
  local destination=$HOME/bk-output-"$shard"
  local logs="$HOME"/bk-debug-"$shard"

  if [ "$no_cache" != "" ]; then
    nerdctl system prune -af
    nerdctl builder prune -af
    rm -Rf "$cache_location"
  fi

  nerdctl build \
    --build-arg UBUNTU_VERSION="$UBUNTU_VERSION" \
    --build-arg CONTAINERD_VERSION="$CONTAINERD_VERSION" \
    --platform="$platform" \
    --output type=tar,dest="$destination" \
    --progress plain \
    --build-arg HTTP_PROXY=$HTTP_PROXY \
    --build-arg HTTPS_PROXY=$HTTPS_PROXY \
    --cache-to type=local,dest="$cache_location",mode=max \
    --cache-from type=local,src="$cache_location" \
    --target "$target" \
    -f "$dockerfile" . 2>&1 | tee "$logs"
}
```

And here is what the CI is doing:

```bash
ci_run(){
  local no_cache="${1:-}"
  export UBUNTU_VERSION=24.04

  CONTAINERD_VERSION=v1.6.36 run "$no_cache" arm64 Dockerfile.origin build-dependencies
  UBUNTU_VERSION=20.04 CONTAINERD_VERSION=v1.6.36 run "" arm64 Dockerfile.origin test-integration

  CONTAINERD_VERSION=v1.7.25 run "$no_cache"  arm64 Dockerfile.origin build-dependencies
  UBUNTU_VERSION=22.04 CONTAINERD_VERSION=v1.7.25 run "" arm64 Dockerfile.origin test-integration

  CONTAINERD_VERSION=v2.0.3 run "$no_cache"  arm64 Dockerfile.origin build-dependencies
  UBUNTU_VERSION=24.04 CONTAINERD_VERSION=v2.0.3 run "" arm64 Dockerfile.origin test-integration

  CONTAINERD_VERSION=v2.0.3 run "$no_cache"  amd64 Dockerfile.origin build-dependencies
  UBUNTU_VERSION=24.04 CONTAINERD_VERSION=v2.0.3 run "" amd64 Dockerfile.origin test-integration
}

# To simulate what happens when there is no cache, go with:
ci_run no_cache

# Once you have a cached run, you can simulate what happens with cache
# First modify something in the nerdctl tree
# Then run it
touch mimick_nerdctl_change
ci_run
```

## Analyzing results

### Network

#### Full CI run, cold cache (the first three pipelines, and part of the fourth)

The following numbers are based on the above script, with cold cache.

Unfortunately golang did segfault on me during the last (cross-run targetting amd), so, these numbers should be taken
as (slightly) underestimated.

Total number of requests: 7190

Total network duration: 13 minutes 11 seconds

Outbound: 1.31MB

Inbound: 5202MB

Breakdown per domain

| Destination                                  | # requests        | through    | duration        |
|----------------------------------------------|-------------------|------------|-----------------|
| https://registry-1.docker.io                 | 123 (2 failed)    | 1.22MB     | 26s             |
| https://production.cloudflare.docker.com     | 60                | 1242.41MB  | 2m6s            |
| http://deb.debian.org                        | 207               | 107.14MB   | 13s             |
| https://github.com                           | 105               | 977.88MB   | 1m25s           |
| https://proxy.golang.org                     | 5343 (57 failed)  | 753.69MB   | 4m8s            |
| https://objects.githubusercontent.com        | 42                | 900.22MB   | 50s             |
| https://raw.githubusercontent.com            | 8                 | 92KB       | 2s              |
| https://storage.googleapis.com               | 19 (3 failed)     | 537.21MB   | 35s             |
| https://ghcr.io                              | 65                | 588.68KB   | 13s             |
| https://auth.docker.io                       | 10                | 259KB      | 5s              |
| https://pkg-containers.githubusercontent.com | 48                | 183.63MB   | 20s             |
| http://ports.ubuntu.com                      | 300               | 165.36MB   | 1m55s           |
| https://golang.org                           | 4                 | 228.93KB   | <1s             |
| https://go.dev                               | 4                 | 95.51KB    | <1s             |
| https://dl.google.com                        | 4                 | 271.42MB   | 11s             |
| https://sum.golang.org                       | 746               | 3.89MB     | 17s             |
| http://security.ubuntu.com                   | 7                 | 2.70MB     | 3s              |
| http://archive.ubuntu.com                    | 95                | 55.95MB    | 19s             |
|                                              | -                 | -          | -               |
| Total                                        | 7190              | 5203MB     | 13 mins 11 secs |


#### Full CI run, warm cache (only the first three pipelines)

| Destination                              | # requests       | through | duration       |
|------------------------------------------|------------------|---------|----------------|
| https://registry-1.docker.io             | 25               | 537KB   | 14s            |
| https://production.cloudflare.docker.com | 2                | 25MB    | 1s             |
| https://github.com                       | 7 (1 failed)     | 105KB   | 2s             |
| https://proxy.golang.org                 | 930 (11 failed)  | 150MB   | 37s            |
| https://objects.githubusercontent.com    | 4                | 86MB    | 4s             |
| https://storage.googleapis.com           | 3                | 112MB   | 6s             |
| https://auth.docker.io                   | 1                | 26KB    | <1s            |
| http://ports.ubuntu.com                  | 133              | 67MB    | 50s            |
| https://golang.org                       | 2                | 114KB   | <1s            |
| https://go.dev                           | 2                | 45KB    | <1s            |
| https://dl.google.com                    | 2                | 134MB   | 5s             |
| https://sum.golang.org                   | 484              | 3MB     | 11s            |
|                                          | -                | -       | -              |
| Total                                    | 1595 (12 failed) | 579MB   | 2 mins 10 secs |


#### Analysis

##### Docker Hub

Images from Docker Hub are clearly a source of concern (made even worse by the fact they apply strict limitations on the
number of requests permitted).

When the cache is cold, this is about 1GB per run, for 200 requests and 3 minutes.

Actions:
- [ ] reduce the number of images
  - we currently use 2 golang images, which does not make sense
- [ ] reduce the round trips
  - there is no reason why any of the images should be queried more than once per build
- [ ] move away from Hub golang image, and instead use a raw distro + golang download
  - Hub golang is a source of pain and issues (diverging version scheme forces ugly shell contorsions, delay in availability creates
broken situations)
  - we are already downloading the go release tarball anyhow, so, this is just wasted bandwidth with no added value

Success criteria:
- on a cold cache, reduce the total number of requests against Docker properties by 50% or more
- on a cold cache, cut the data transfer and time in half

##### Distro packages

On a WARM cache, close to 1 minute is spent fetching Ubuntu packages.
This should not happen, and distro downloads should always be cached.

On a cold cache, distro packages download near 3 minutes.
Very likely there is stage duplication that could be reduced and some of that could be cut of.

Actions:
- [ ] ensure distro package downloading is staged in a way we can cache it
- [ ] review stages to reduce package installation duplication

Success criteria:
- [ ] 0 package installation on a warm cache
- [ ] cut cold cache package install time by 50% (XXX not realistic?)


##### GitHub repositories

Clones from GitHub do clock in at 1GB on a cold cache.
Containerd alone counts for more than half of it (at 160MB+ x4).

Hopefully, on a warm cache it is practically non-existent.

But then, this is ridiculous.

Actions:
- [ ] shallow clone

Success criteria:
- [ ] reduce network traffic from cloning by 80%

##### Go modules

At 750+MB and over 4 minutes, this is the number one speed bottleneck on a cold cache.

On a warm cache, it is still over 150MB and 30+ seconds.

In and of itself, this is hard to reduce, as we need these...

Actions:
- [ ] we could cache the module download location to reduce round-trips on modules that are shared accross
different projects
- [ ] we are likely installing nerdctl modules six times - (once per architecture during the build phase, then once per
ubuntu version and architecture during the tests runs (this is not even accounted for in the audit above)) - it should
only happen twice (once per architecture)

Success criteria:
- [ ] achieve 20% reduction of total time spent downloading go modules

##### Other downloads

1. At 500MB+ and 30 seconds, storage.googleapis.com is serving a SINGLE go module that gets special treatment: klauspost/compress.
This module is very small, but does serve along with it a very large `testdata` folder.
The fact that nerdctl downloads its module multiple times is further compounding the effect.

2. the golang archive is downloaded multiple times - it should be downloaded only once per run, and only on a cold cache

3. some of the binary releases we are retrieving are also being retrieved with a warm cache, and they are generally quite large.
We could consider building certain things from source instead, and in all cases ensure that we are only downloading with a cold cache.

Success criteria:
- [ ] 0 static downloads on a warm cache
- [ ] cut extra downloads by 20%

#### Duration

Unscientific numbers, per pipeline

dependencies, no cache:
- 224 seconds total
- 53 seconds exporting cache

dependencies, with cache:
- 12 seconds

test-integration, no cache:
- 282 seconds

#### Caching

Number of layers in cache:
```
after dependencies stage: 78
intermediate size: 1.5G
after test-integration stage: 118
total size: 2.8G
```

## Generic considerations

### Caching compression

This is obviously heavily dependent on the runner properties.

With local cache, on high-performance IO (laptop SSD), zstd is definitely considerably better (about twice as fast).

With GHA, the impact is minimal, since network IO is heavily dominant, but zstd still has the upper
hand with regard to cache size.

### Output

Loading image into the Docker store comes at a somewhat significant cost.
It is quite possible that a significant performance boost could be achieved by using
buildkit containerd worker and nerdctl instead.
