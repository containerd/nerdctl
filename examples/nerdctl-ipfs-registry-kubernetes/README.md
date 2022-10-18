# Examples of Node-to-Node image sharing on Kubernetes using `nerdctl ipfs registry`

This directory contains examples of node-to-node image sharing on Kubernetes with `nerdctl ipfs registry`.

- [`./ipfs`](./ipfs): node-to-node image sharing using IPFS
- [`./ipfs-cluster`](./ipfs-cluster): node-to-node image sharing with content replication using ipfs-cluster
- [`./ipfs-stargz-snapshotter`](./ipfs-stargz-snapshotter): node-to-node image sharing with lazy pulling using eStargz and Stargz Snapshotter

## Example Dockerfile of `nerdctl ipfs regisry`

The above examples use `nerdctl ipfs regisry` running in a Pod.
The image is available at [`ghcr.io/stargz-containers/nerdctl-ipfs-registry`](https://github.com/orgs/stargz-containers/packages/container/package/nerdctl-ipfs-registry).

The following Dockerfile can be used to build it by yourself.

```Dockerfile
FROM ubuntu:22.04 AS dev

ARG NERDCTL_VERSION=0.23.0
ARG NERDCTL_AMD64_SHA256SUM=aa00cd197de3549469e9c62753798979203dc0607f3e60f119ed632478244553
ARG NERDCTL_ARM64_SHA256SUM=bc8095b8d60a2f25da7e5c456705dce2db020a0a87d003093550994618189ea3
ARG NERDCTL_PPC64LE_SHA256SUM=162d68a636e0a9c32f705f27390ae8ed919ca8c0442832909ebf3c0e5a884fac
ARG NERDCTL_RISCV64_SHA256SUM=1580fe87e730fe4b4442ce3e5199fa487ca03dcd0761f0bfa3c7603e4be10372
ARG NERDCTL_S390X_SHA256SUM=7905ef258968c6f331944a097afe28251c793471fdbc4b7e87aae63f999e8098

RUN apt-get update -y && apt-get install -y curl && \
    curl -sSL --output /tmp/nerdctl.${TARGETARCH:-amd64}.tgz https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-${TARGETARCH:-amd64}.tar.gz && \
    echo "${NERDCTL_AMD64_SHA256SUM}  /tmp/nerdctl.amd64.tgz" | tee /tmp/nerdctl.sha256 && \
    echo "${NERDCTL_ARM64_SHA256SUM}  /tmp/nerdctl.arm64.tgz" | tee -a /tmp/nerdctl.sha256 && \
    echo "${NERDCTL_PPC64LE_SHA256SUM}  /tmp/nerdctl.ppc64le.tgz" | tee -a /tmp/nerdctl.sha256 && \
    echo "${NERDCTL_RISCV64_SHA256SUM}  /tmp/nerdctl.riscv64.tgz" | tee -a /tmp/nerdctl.sha256 && \
    echo "${NERDCTL_S390X_SHA256SUM}  /tmp/nerdctl.s390x.tgz" | tee -a /tmp/nerdctl.sha256 && \
    sha256sum --ignore-missing -c /tmp/nerdctl.sha256 && \
    tar zxvf /tmp/nerdctl.${TARGETARCH:-amd64}.tgz -C /usr/local/bin/ && \
    rm /tmp/nerdctl.${TARGETARCH:-amd64}.tgz

FROM ubuntu:22.04
COPY --from=dev /usr/local/bin/nerdctl /usr/local/bin/nerdctl
ENTRYPOINT [ "/usr/local/bin/nerdctl", "ipfs", "registry", "serve" ]
```
