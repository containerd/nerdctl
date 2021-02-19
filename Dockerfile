# Usage: `docker run -it --privileged <IMAGE>`. Make sure to add `-t` and `--privileged`.
ARG UBUNTU_VERSION=20.04
ARG CONTAINERIZED_SYSTEMD_VERSION=0.1.0
ARG CONTAINERD_VERSION=1.4.3
ARG RUNC_VERSION=1.0.0-rc92
ARG CNI_PLUGINS_VERSION=0.9.0
ARG CNI_ISOLATION_VERSION=0.0.3
ARG BUILDKIT_VERSION=0.8.1
ARG GO_VERSION=1.15.8

FROM ubuntu:${UBUNTU_VERSION} AS base
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get install -qq -y --no-install-recommends \
  ca-certificates curl \
  iproute2 iptables \
  dbus systemd systemd-sysv
ARG CONTAINERIZED_SYSTEMD_VERSION
RUN curl -L -o /docker-entrypoint.sh https://raw.githubusercontent.com/AkihiroSuda/containerized-systemd/v${CONTAINERIZED_SYSTEMD_VERSION}/docker-entrypoint.sh && \
  chmod +x /docker-entrypoint.sh
ARG CONTAINERD_VERSION
ARG TARGETARCH
RUN curl -L https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /usr/local && \
  rm -f /usr/local/bin/containerd-{shim,shim-runc-v1}
COPY Dockerfile.d/containerd.service /etc/systemd/system/containerd.service
RUN systemctl enable containerd
ARG RUNC_VERSION
RUN curl -L -o /usr/local/sbin/runc https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.${TARGETARCH:-amd64} && \
  chmod +x /usr/local/sbin/runc
ARG CNI_PLUGINS_VERSION
RUN mkdir -p /opt/cni/bin && \
  curl -L https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/cni-plugins-linux-${TARGETARCH:-amd64}-v${CNI_PLUGINS_VERSION}.tgz | tar xzvC /opt/cni/bin
ARG CNI_ISOLATION_VERSION
RUN curl -L https://github.com/AkihiroSuda/cni-isolation/releases/download/v${CNI_ISOLATION_VERSION}/cni-isolation-${TARGETARCH:-amd64}.tgz | tar xzvC /opt/cni/bin
ARG BUILDKIT_VERSION
RUN curl -L https://github.com/moby/buildkit/releases/download/v${BUILDKIT_VERSION}/buildkit-v${BUILDKIT_VERSION}.linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /usr/local && \
  rm -f /usr/local/bin/buildkit-qemu-* /usr/local/bin/buildkit-runc
COPY Dockerfile.d/buildkitd.service /etc/systemd/system/buildkitd.service
RUN systemctl enable buildkitd
VOLUME /var/lib/containerd
VOLUME /var/lib/nerdctl
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["bash"]

FROM base AS test
RUN apt-get update && \
  apt-get install -qq -y \
  make git
ARG GO_VERSION
ARG TARGETARCH
RUN curl -L https://golang.org/dl/go${GO_VERSION}.linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /usr/local
ENV PATH=/usr/local/go/bin:$PATH
COPY . /go/src/github.com/AkihiroSuda/nerdctl
WORKDIR /go/src/github.com/AkihiroSuda/nerdctl
ENV CGO_ENABLED=0
RUN make binaries install
CMD ["go", "test", "-v", "./..."]

FROM base AS demo
COPY --from=test /usr/local/bin/nerdctl /usr/local/bin
