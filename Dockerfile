# Usage: `docker run -it --privileged <IMAGE>`. Make sure to add `-t` and `--privileged`.

# Basic deps
ARG CONTAINERD_VERSION=1.5.0-beta.2
ARG RUNC_VERSION=1.0.0-rc93
ARG CNI_PLUGINS_VERSION=0.9.1

# Extra deps: CNI isolation
ARG CNI_ISOLATION_VERSION=0.0.3
# Extra deps: Build
ARG BUILDKIT_VERSION=0.8.2
# Extra deps: Lazy-pulling
ARG STARGZ_SNAPSHOTTER_VERSION=0.4.1
# Extra deps: Encryption
ARG IMGCRYPT_VERSION=1.1.0
# Extra deps: Rootless
ARG ROOTLESSKIT_VERSION=0.14.0-beta.0
ARG SLIRP4NETNS_VERSION=1.1.9
# Extra deps: FUSE-OverlayFS
ARG FUSE_OVERLAYFS_VERSION=1.4.0
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION=1.0.1

# Test deps
ARG GO_VERSION=1.16
ARG UBUNTU_VERSION=20.04
ARG CONTAINERIZED_SYSTEMD_VERSION=0.1.1

FROM golang:${GO_VERSION}-alpine AS build-minimal
RUN apk add --no-cache make git
COPY . /go/src/github.com/AkihiroSuda/nerdctl
WORKDIR /go/src/github.com/AkihiroSuda/nerdctl
RUN BINDIR=/out/bin make binaries install
# We do not set CMD to `go test` here, because it requires systemd

FROM build-minimal AS build-full
RUN apk add --no-cache curl
RUN mkdir -p /out/share/doc/nerdctl-full && \
  echo "# nerdctl (full distribution)" > /out/share/doc/nerdctl-full/README.md && \
  echo "- nerdctl: $(cd /go/src/github.com/AkihiroSuda/nerdctl && git describe --tags)" >> /out/share/doc/nerdctl-full/README.md
ARG TARGETARCH
ARG CONTAINERD_VERSION
RUN curl -L https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /out && \
  rm -f /out/bin/containerd-shim /out/bin/containerd-shim-runc-v1 && \
  mkdir -p /out/lib/systemd/system && \
  curl -L -o /out/lib/systemd/system/containerd.service https://raw.githubusercontent.com/containerd/containerd/v${CONTAINERD_VERSION}/containerd.service && \
  echo "- containerd: v${CONTAINERD_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG RUNC_VERSION
RUN curl -L -o /out/bin/runc https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.${TARGETARCH:-amd64} && \
  chmod +x /out/bin/runc && \
  echo "- runc: v${RUNC_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CNI_PLUGINS_VERSION
RUN mkdir -p /out/libexec/cni && \
  curl -L https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/cni-plugins-linux-${TARGETARCH:-amd64}-v${CNI_PLUGINS_VERSION}.tgz | tar xzvC /out/libexec/cni && \
  echo "- CNI plugins: v${CNI_PLUGINS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CNI_ISOLATION_VERSION
RUN curl -L https://github.com/AkihiroSuda/cni-isolation/releases/download/v${CNI_ISOLATION_VERSION}/cni-isolation-${TARGETARCH:-amd64}.tgz | tar xzvC /out/libexec/cni && \
  echo "- CNI isolation plugin: v${CNI_ISOLATION_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG BUILDKIT_VERSION
RUN curl -L https://github.com/moby/buildkit/releases/download/v${BUILDKIT_VERSION}/buildkit-v${BUILDKIT_VERSION}.linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /out && \
  rm -f /out/bin/buildkit-qemu-* /out/bin/buildkit-runc && \
  echo "- BuildKit: v${BUILDKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
# NOTE: github.com/moby/buildkit/examples/systemd is not included in BuildKit v0.8.x, will be included in v0.9.x
RUN cd /out/lib/systemd/system && \
  sedcomm='s@bin/containerd@bin/buildkitd@g; s@(Description|Documentation)=.*@@' && \
  sed -E "${sedcomm}" containerd.service > buildkit.service && \
  echo "" >> buildkit.service && \
  echo "# This file was converted from containerd.service, with \`sed -E '${sedcomm}'\`" >> buildkit.service
ARG STARGZ_SNAPSHOTTER_VERSION
RUN curl -L https://github.com/containerd/stargz-snapshotter/releases/download/v${STARGZ_SNAPSHOTTER_VERSION}/stargz-snapshotter-v${STARGZ_SNAPSHOTTER_VERSION}-linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /out/bin && \
  curl -L -o /out/lib/systemd/system/stargz-snapshotter.service https://raw.githubusercontent.com/containerd/stargz-snapshotter/v${STARGZ_SNAPSHOTTER_VERSION}/script/config/etc/systemd/system/stargz-snapshotter.service && \
  echo "- Stargz Snapshotter: v${STARGZ_SNAPSHOTTER_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG IMGCRYPT_VERSION
RUN git clone https://github.com/containerd/imgcrypt.git /go/src/github.com/containerd/imgcrypt && \
  cd /go/src/github.com/containerd/imgcrypt && \
  CGO_ENABLED=0 make && DESTDIR=/out make install && \
  echo "- imgcrypt: v${IMGCRYPT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG ROOTLESSKIT_VERSION
RUN curl -L https://github.com/rootless-containers/rootlesskit/releases/download/v${ROOTLESSKIT_VERSION}/rootlesskit-$(uname -m).tar.gz | tar xzvC /out/bin && \
  rm -f /out/bin/rootlesskit-docker-proxy && \
  echo "- RootlessKit: v${ROOTLESSKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG SLIRP4NETNS_VERSION
RUN curl -L -o /out/bin/slirp4netns https://github.com/rootless-containers/slirp4netns/releases/download/v${SLIRP4NETNS_VERSION}/slirp4netns-$(uname -m) && \
  chmod +x /out/bin/slirp4netns && \
  echo "- slirp4netns: v${SLIRP4NETNS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG FUSE_OVERLAYFS_VERSION
RUN curl -L -o /out/bin/fuse-overlayfs https://github.com/containers/fuse-overlayfs/releases/download/v${FUSE_OVERLAYFS_VERSION}/fuse-overlayfs-$(uname -m) && \
  chmod +x /out/bin/fuse-overlayfs && \
  echo "- fuse-overlayfs: v${FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION
RUN curl -L https://github.com/AkihiroSuda/containerd-fuse-overlayfs/releases/download/v${CONTAINERD_FUSE_OVERLAYFS_VERSION}/containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION}-linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /out/bin && \
  echo "- containerd-fuse-overlayfs: v${CONTAINERD_FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
RUN echo "" >> /out/share/doc/nerdctl-full/README.md && \
  echo "## License" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/slirp4netns:    [GNU GENERAL PUBLIC LICENSE, Version 2](https://github.com/rootless-containers/slirp4netns/blob/v${SLIRP4NETNS_VERSION}/COPYING)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/fuse-overlayfs: [GNU GENERAL PUBLIC LICENSE, Version 3](https://github.com/containers/fuse-overlayfs/blob/v${FUSE_OVERLAYFS_VERSION}/COPYING)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- Other files: [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0)" >> /out/share/doc/nerdctl-full/README.md
RUN (cd /out && find ! -type d | sort | xargs sha256sum > /tmp/SHA256SUMS ) && \
  mv /tmp/SHA256SUMS /out/share/doc/nerdctl-full/SHA256SUMS

FROM scratch AS out-full
COPY --from=build-full /out /

FROM ubuntu:${UBUNTU_VERSION} AS base
# fuse3 is required by stargz snapshotter
RUN apt-get update && \
  apt-get install -qq -y --no-install-recommends \
  ca-certificates curl \
  iproute2 iptables \
  dbus systemd systemd-sysv \
  fuse3
ARG CONTAINERIZED_SYSTEMD_VERSION
RUN curl -L -o /docker-entrypoint.sh https://raw.githubusercontent.com/AkihiroSuda/containerized-systemd/v${CONTAINERIZED_SYSTEMD_VERSION}/docker-entrypoint.sh && \
  chmod +x /docker-entrypoint.sh
COPY --from=out-full / /usr/local/
RUN perl -pi -e 's/multi-user.target/docker-entrypoint.target/g' /usr/local/lib/systemd/system/*.service && \
  systemctl enable containerd buildkit stargz-snapshotter
COPY ./Dockerfile.d/etc_containerd_config.toml /etc/containerd/config.toml
VOLUME /var/lib/containerd
VOLUME /var/lib/buildkit
VOLUME /var/lib/containerd-stargz-grpc
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
CMD ["go", "test", "-v", "./..."]

FROM test AS test-rootless
RUN apt-get update && \
  apt-get install -qq -y \
  dbus-user-session systemd-container uidmap
RUN useradd -m -s /bin/bash rootless && \
  mkdir -p /home/rootless/.local/share && \
  chown -R rootless:rootless /home/rootless
VOLUME /home/rootless/.local/share
RUN go test -o /usr/local/bin/nerdctl.test -c .
CMD ["machinectl", "shell", "rootless@", "/bin/sh", "-euxc", \
  "containerd-rootless-setuptool.sh install && containerd-rootless-setuptool.sh install-buildkit && exec nerdctl.test -test.v -test.kill-daemon"]

FROM base AS demo
