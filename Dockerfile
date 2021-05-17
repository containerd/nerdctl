#   Copyright The containerd Authors.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

# -----------------------------------------------------------------------------
# Usage: `docker run -it --privileged <IMAGE>`. Make sure to add `-t` and `--privileged`.

# Basic deps
ARG CONTAINERD_VERSION=1.5.1
ARG RUNC_VERSION=1.0.0-rc94
ARG CNI_PLUGINS_VERSION=0.9.1

# Extra deps: CNI isolation
ARG CNI_ISOLATION_VERSION=0.0.3
# Extra deps: Build
ARG BUILDKIT_VERSION=0.8.3
# Extra deps: Lazy-pulling
ARG STARGZ_SNAPSHOTTER_VERSION=0.6.0
# Extra deps: Encryption
ARG IMGCRYPT_VERSION=1.1.1
# Extra deps: Rootless
ARG ROOTLESSKIT_VERSION=0.14.2
ARG SLIRP4NETNS_VERSION=1.1.9
# Extra deps: FUSE-OverlayFS
ARG FUSE_OVERLAYFS_VERSION=1.5.0
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION=1.0.2

# Test deps
ARG GO_VERSION=1.16
ARG UBUNTU_VERSION=20.04
ARG CONTAINERIZED_SYSTEMD_VERSION=0.1.1

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-buster AS build-runc
RUN dpkg --add-architecture arm64 && \
  apt-get update && \
  apt-get install -y crossbuild-essential-arm64 git libseccomp-dev libseccomp-dev:arm64
ARG RUNC_VERSION
RUN git clone https://github.com/opencontainers/runc.git /go/src/github.com/opencontainers/runc
WORKDIR /go/src/github.com/opencontainers/runc
RUN git checkout v${RUNC_VERSION} && \
  mkdir -p /out
RUN GOARCH=amd64 CC=x86_64-linux-gnu-gcc make runc && \
  cp -a runc /out/runc.amd64
RUN GOARCH=arm64 CC=aarch64-linux-gnu-gcc make runc && \
  cp -a runc /out/runc.arm64

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build-base
RUN apk add --no-cache make git curl
COPY . /go/src/github.com/containerd/nerdctl
WORKDIR /go/src/github.com/containerd/nerdctl

FROM build-base AS build-minimal
RUN BINDIR=/out/bin make binaries install
# We do not set CMD to `go test` here, because it requires systemd

FROM build-base AS build-full
ARG TARGETARCH
ENV GOARCH=${TARGETARCH}
RUN BINDIR=/out/bin make binaries install
WORKDIR /nowhere
COPY ./Dockerfile.d/SHA256SUMS.d/ /SHA256SUMS.d
COPY README.md /out/share/doc/nerdctl/
COPY docs /out/share/doc/nerdctl/docs
RUN echo "${TARGETARCH:-amd64}" | sed -e s/amd64/x86_64/ -e s/arm64/aarch64/ | tee /target_uname_m
RUN mkdir -p /out/share/doc/nerdctl-full && \
  echo "# nerdctl (full distribution)" > /out/share/doc/nerdctl-full/README.md && \
  echo "- nerdctl: $(cd /go/src/github.com/containerd/nerdctl && git describe --tags)" >> /out/share/doc/nerdctl-full/README.md
ARG CONTAINERD_VERSION
RUN fname="containerd-${CONTAINERD_VERSION}.${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/kind-ci/containerd-nightlies/releases/download/containerd-${CONTAINERD_VERSION}/${fname}" && \
  curl -o "containerd.service" -fSL "https://raw.githubusercontent.com/containerd/containerd/v${CONTAINERD_VERSION}/containerd.service" && \
  grep "${fname}" "/SHA256SUMS.d/containerd-${CONTAINERD_VERSION}" | sha256sum -c - && \
  grep "containerd.service" "/SHA256SUMS.d/containerd-${CONTAINERD_VERSION}" | sha256sum -c - && \
  tar xzf "${fname}" -C /out && \
  rm -f "${fname}" /out/bin/containerd-shim /out/bin/containerd-shim-runc-v1 && \
  mkdir -p /out/lib/systemd/system && \
  mv containerd.service /out/lib/systemd/system/containerd.service && \
  echo "- containerd: v${CONTAINERD_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG RUNC_VERSION
COPY --from=build-runc /out/runc.${TARGETARCH:-amd64} /out/bin/runc
RUN echo "- runc: v${RUNC_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CNI_PLUGINS_VERSION
RUN fname="cni-plugins-${TARGETOS:-linux}-${TARGETARCH:-amd64}-v${CNI_PLUGINS_VERSION}.tgz" && \
  curl -o "${fname}" -fSL "https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/cni-plugins-${CNI_PLUGINS_VERSION}" | sha256sum -c && \
  mkdir -p /out/libexec/cni && \
  tar xzf "${fname}" -C /out/libexec/cni && \
  rm -f "${fname}" && \
  echo "- CNI plugins: v${CNI_PLUGINS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CNI_ISOLATION_VERSION
RUN fname="cni-isolation-${TARGETARCH:-amd64}.tgz" && \
  curl -o "${fname}" -fSL "https://github.com/AkihiroSuda/cni-isolation/releases/download/v${CNI_ISOLATION_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/cni-isolation-${CNI_ISOLATION_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/libexec/cni && \
  rm -f "${fname}" && \
  echo "- CNI isolation plugin: v${CNI_ISOLATION_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG BUILDKIT_VERSION
RUN fname="buildkit-v${BUILDKIT_VERSION}.${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/moby/buildkit/releases/download/v${BUILDKIT_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/buildkit-${BUILDKIT_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out && \
  rm -f "${fname}" /out/bin/buildkit-qemu-* /out/bin/buildkit-runc && \
  echo "- BuildKit: v${BUILDKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
# NOTE: github.com/moby/buildkit/examples/systemd is not included in BuildKit v0.8.x, will be included in v0.9.x
RUN cd /out/lib/systemd/system && \
  sedcomm='s@bin/containerd@bin/buildkitd@g; s@(Description|Documentation)=.*@@' && \
  sed -E "${sedcomm}" containerd.service > buildkit.service && \
  echo "" >> buildkit.service && \
  echo "# This file was converted from containerd.service, with \`sed -E '${sedcomm}'\`" >> buildkit.service
ARG STARGZ_SNAPSHOTTER_VERSION
RUN fname="stargz-snapshotter-v${STARGZ_SNAPSHOTTER_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/containerd/stargz-snapshotter/releases/download/v${STARGZ_SNAPSHOTTER_VERSION}/${fname}" && \
  curl -o "stargz-snapshotter.service" -fSL "https://raw.githubusercontent.com/containerd/stargz-snapshotter/v${STARGZ_SNAPSHOTTER_VERSION}/script/config/etc/systemd/system/stargz-snapshotter.service" && \
  grep "${fname}" "/SHA256SUMS.d/stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}" | sha256sum -c - && \
  grep "stargz-snapshotter.service" "/SHA256SUMS.d/stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}" | sha256sum -c - && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" && \
  mv stargz-snapshotter.service /out/lib/systemd/system/stargz-snapshotter.service && \
  echo "- Stargz Snapshotter: v${STARGZ_SNAPSHOTTER_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG IMGCRYPT_VERSION
RUN git clone https://github.com/containerd/imgcrypt.git /go/src/github.com/containerd/imgcrypt && \
  cd /go/src/github.com/containerd/imgcrypt && \
  CGO_ENABLED=0 make && DESTDIR=/out make install && \
  echo "- imgcrypt: v${IMGCRYPT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG ROOTLESSKIT_VERSION
RUN fname="rootlesskit-$(cat /target_uname_m).tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/rootless-containers/rootlesskit/releases/download/v${ROOTLESSKIT_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/rootlesskit-${ROOTLESSKIT_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" /out/bin/rootlesskit-docker-proxy && \
  echo "- RootlessKit: v${ROOTLESSKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG SLIRP4NETNS_VERSION
RUN fname="slirp4netns-$(cat /target_uname_m)" && \
  curl -o "${fname}" -fSL "https://github.com/rootless-containers/slirp4netns/releases/download/v${SLIRP4NETNS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/slirp4netns-${SLIRP4NETNS_VERSION}" | sha256sum -c && \
  mv "${fname}" /out/bin/slirp4netns && \
  chmod +x /out/bin/slirp4netns && \
  echo "- slirp4netns: v${SLIRP4NETNS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG FUSE_OVERLAYFS_VERSION
RUN fname="fuse-overlayfs-$(cat /target_uname_m)" && \
  curl -o "${fname}" -fSL "https://github.com/containers/fuse-overlayfs/releases/download/v${FUSE_OVERLAYFS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/fuse-overlayfs-${FUSE_OVERLAYFS_VERSION}" | sha256sum -c && \
  mv "${fname}" /out/bin/fuse-overlayfs && \
  chmod +x /out/bin/fuse-overlayfs && \
  echo "- fuse-overlayfs: v${FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION
RUN fname="containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/containerd/fuse-overlayfs-snapshotter/releases/download/v${CONTAINERD_FUSE_OVERLAYFS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" && \
  echo "- containerd-fuse-overlayfs: v${CONTAINERD_FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
RUN echo "" >> /out/share/doc/nerdctl-full/README.md && \
  echo "## License" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/slirp4netns:    [GNU GENERAL PUBLIC LICENSE, Version 2](https://github.com/rootless-containers/slirp4netns/blob/v${SLIRP4NETNS_VERSION}/COPYING)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/fuse-overlayfs: [GNU GENERAL PUBLIC LICENSE, Version 3](https://github.com/containers/fuse-overlayfs/blob/v${FUSE_OVERLAYFS_VERSION}/COPYING)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- Other files: [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0)" >> /out/share/doc/nerdctl-full/README.md && \
  (cd /out && find ! -type d | sort | xargs sha256sum > /tmp/SHA256SUMS ) && \
  mv /tmp/SHA256SUMS /out/share/doc/nerdctl-full/SHA256SUMS && \
  chown -R 0:0 /out

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

# convert GO_VERSION=1.16 to the latest release such as "go1.16.1"
FROM golang:${GO_VERSION}-alpine AS goversion
RUN go env GOVERSION > /GOVERSION

FROM base AS test
# `expect` package contains `unbuffer(1)`, which is used for emulating TTY for testing
RUN apt-get update && \
  apt-get install -qq -y \
  expect
COPY --from=goversion /GOVERSION /GOVERSION
ARG TARGETARCH
RUN curl -L https://golang.org/dl/$(cat /GOVERSION).linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /usr/local
ENV PATH=/usr/local/go/bin:$PATH
COPY . /go/src/github.com/containerd/nerdctl
WORKDIR /go/src/github.com/containerd/nerdctl
VOLUME /tmp
ENV CGO_ENABLED=0
CMD ["go", "test", "-v", "./..."]

FROM test AS test-rootless
# Install SSH for creating systemd user session.
# (`sudo` does not work for this purpose,
#  OTOH `machinectl shell` can create the session but does not propagate exit code)
RUN apt-get update && \
  apt-get install -qq -y \
  uidmap \
  dbus-user-session \
  openssh-server openssh-client
# TODO: update containerized-systemd to enable sshd by default, or allow `systemctl wants <TARGET> sshd` here
RUN ssh-keygen -q -t rsa -f /root/.ssh/id_rsa -N '' && \
  useradd -m -s /bin/bash rootless && \
  mkdir -p -m 0700 /home/rootless/.ssh && \
  cp -a /root/.ssh/id_rsa.pub /home/rootless/.ssh/authorized_keys && \
  mkdir -p /home/rootless/.local/share && \
  chown -R rootless:rootless /home/rootless
VOLUME /home/rootless/.local/share
RUN go test -o /usr/local/bin/nerdctl.test -c .
COPY ./Dockerfile.d/test-rootless.sh /
CMD ["/test-rootless.sh", "nerdctl.test" ,"-test.v", "-test.kill-daemon"]

# test for CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER=slirp4netns
FROM test-rootless AS test-rootless-port-slirp4netns
COPY ./Dockerfile.d/home_rootless_.config_systemd_user_containerd.service.d_port-slirp4netns.conf /home/rootless/.config/systemd/user/containerd.service.d/port-slirp4netns.conf
RUN chown -R rootless:rootless /home/rootless/.config

FROM base AS demo
