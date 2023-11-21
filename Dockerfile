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

# TODO: verify commit hash

# Basic deps
ARG CONTAINERD_VERSION=v1.7.8
ARG RUNC_VERSION=v1.1.10
ARG CNI_PLUGINS_VERSION=v1.3.0

# Extra deps: Build
ARG BUILDKIT_VERSION=v0.12.3
# Extra deps: Lazy-pulling
ARG STARGZ_SNAPSHOTTER_VERSION=v0.15.1
# Extra deps: Encryption
ARG IMGCRYPT_VERSION=v1.1.9
# Extra deps: Rootless
ARG ROOTLESSKIT_VERSION=v1.1.1
ARG SLIRP4NETNS_VERSION=v1.2.2
# Extra deps: bypass4netns
ARG BYPASS4NETNS_VERSION=v0.3.0
# Extra deps: FUSE-OverlayFS
ARG FUSE_OVERLAYFS_VERSION=v1.13
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION=v1.0.8
# Extra deps: IPFS
ARG KUBO_VERSION=v0.23.0
# Extra deps: Init
ARG TINI_VERSION=v0.19.0
# Extra deps: Debug
ARG BUILDG_VERSION=v0.4.1

# Test deps
ARG GO_VERSION=1.21
ARG UBUNTU_VERSION=22.04
ARG CONTAINERIZED_SYSTEMD_VERSION=v0.1.1
ARG GOTESTSUM_VERSION=v1.11.0
ARG NYDUS_VERSION=v2.2.3
ARG SOCI_SNAPSHOTTER_VERSION=0.4.0

FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.3.0 AS xx


FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bullseye AS build-base-debian
COPY --from=xx / /
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
  apt-get install -y git pkg-config dpkg-dev
ARG TARGETARCH
# libbtrfs: for containerd
# libseccomp: for runc and bypass4netns
RUN xx-apt-get update && \
  xx-apt-get install -y binutils gcc libc6-dev libbtrfs-dev libseccomp-dev

FROM build-base-debian AS build-containerd
ARG TARGETARCH
ARG CONTAINERD_VERSION
RUN git clone https://github.com/containerd/containerd.git /go/src/github.com/containerd/containerd
WORKDIR /go/src/github.com/containerd/containerd
RUN git checkout ${CONTAINERD_VERSION} && \
  mkdir -p /out /out/$TARGETARCH && \
  cp -a containerd.service /out
RUN GO=xx-go make STATIC=1 && \
  cp -a bin/containerd bin/containerd-shim-runc-v2 bin/ctr /out/$TARGETARCH

FROM build-base-debian AS build-runc
ARG RUNC_VERSION
ARG TARGETARCH
RUN git clone https://github.com/opencontainers/runc.git /go/src/github.com/opencontainers/runc
WORKDIR /go/src/github.com/opencontainers/runc
RUN git checkout ${RUNC_VERSION} && \
  mkdir -p /out
ENV CGO_ENABLED=1
RUN GO=xx-go make static && \
  xx-verify --static runc && cp -v -a runc /out/runc.${TARGETARCH}

FROM build-base-debian AS build-bypass4netns
ARG BYPASS4NETNS_VERSION
ARG TARGETARCH
RUN git clone https://github.com/rootless-containers/bypass4netns.git /go/src/github.com/rootless-containers/bypass4netns
WORKDIR /go/src/github.com/rootless-containers/bypass4netns
RUN git checkout ${BYPASS4NETNS_VERSION} && \
  mkdir -p /out/${TARGETARCH}
ENV CGO_ENABLED=1
RUN GO=xx-go make static && \
  xx-verify --static bypass4netns && cp -a bypass4netns bypass4netnsd /out/${TARGETARCH}

FROM build-base-debian AS build-kubo
ARG KUBO_VERSION
ARG TARGETARCH
RUN git clone https://github.com/ipfs/kubo.git /go/src/github.com/ipfs/kubo
WORKDIR /go/src/github.com/ipfs/kubo
RUN git checkout ${KUBO_VERSION} && \
  mkdir -p /out/${TARGETARCH}
ENV CGO_ENABLED=0
RUN xx-go --wrap && \
  make build && \
  xx-verify --static cmd/ipfs/ipfs && cp -a cmd/ipfs/ipfs /out/${TARGETARCH}

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
COPY --from=build-containerd /out/${TARGETARCH:-amd64}/* /out/bin/
COPY --from=build-containerd /out/containerd.service /out/lib/systemd/system/containerd.service
RUN echo "- containerd: ${CONTAINERD_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG RUNC_VERSION
COPY --from=build-runc /out/runc.${TARGETARCH:-amd64} /out/bin/runc
RUN echo "- runc: ${RUNC_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CNI_PLUGINS_VERSION
RUN fname="cni-plugins-${TARGETOS:-linux}-${TARGETARCH:-amd64}-${CNI_PLUGINS_VERSION}.tgz" && \
  curl -o "${fname}" -fSL "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/cni-plugins-${CNI_PLUGINS_VERSION}" | sha256sum -c && \
  mkdir -p /out/libexec/cni && \
  tar xzf "${fname}" -C /out/libexec/cni && \
  rm -f "${fname}" && \
  echo "- CNI plugins: ${CNI_PLUGINS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG BUILDKIT_VERSION
RUN fname="buildkit-${BUILDKIT_VERSION}.${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/moby/buildkit/releases/download/${BUILDKIT_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/buildkit-${BUILDKIT_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out && \
  rm -f "${fname}" /out/bin/buildkit-qemu-* /out/bin/buildkit-runc && \
  echo "- BuildKit: ${BUILDKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
# NOTE: github.com/moby/buildkit/examples/systemd is not included in BuildKit v0.8.x, will be included in v0.9.x
RUN cd /out/lib/systemd/system && \
  sedcomm='s@bin/containerd@bin/buildkitd@g; s@(Description|Documentation)=.*@@' && \
  sed -E "${sedcomm}" containerd.service > buildkit.service && \
  echo "" >> buildkit.service && \
  echo "# This file was converted from containerd.service, with \`sed -E '${sedcomm}'\`" >> buildkit.service
ARG STARGZ_SNAPSHOTTER_VERSION
RUN fname="stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/containerd/stargz-snapshotter/releases/download/${STARGZ_SNAPSHOTTER_VERSION}/${fname}" && \
  curl -o "stargz-snapshotter.service" -fSL "https://raw.githubusercontent.com/containerd/stargz-snapshotter/${STARGZ_SNAPSHOTTER_VERSION}/script/config/etc/systemd/system/stargz-snapshotter.service" && \
  grep "${fname}" "/SHA256SUMS.d/stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}" | sha256sum -c - && \
  grep "stargz-snapshotter.service" "/SHA256SUMS.d/stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}" | sha256sum -c - && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" /out/bin/stargz-store && \
  mv stargz-snapshotter.service /out/lib/systemd/system/stargz-snapshotter.service && \
  echo "- Stargz Snapshotter: ${STARGZ_SNAPSHOTTER_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG IMGCRYPT_VERSION
RUN git clone https://github.com/containerd/imgcrypt.git /go/src/github.com/containerd/imgcrypt && \
  cd /go/src/github.com/containerd/imgcrypt && \
  git checkout "${IMGCRYPT_VERSION}" && \
  CGO_ENABLED=0 make && DESTDIR=/out make install && \
  echo "- imgcrypt: ${IMGCRYPT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG ROOTLESSKIT_VERSION
RUN fname="rootlesskit-$(cat /target_uname_m).tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/rootless-containers/rootlesskit/releases/download/${ROOTLESSKIT_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/rootlesskit-${ROOTLESSKIT_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" /out/bin/rootlesskit-docker-proxy && \
  echo "- RootlessKit: ${ROOTLESSKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG SLIRP4NETNS_VERSION
RUN fname="slirp4netns-$(cat /target_uname_m)" && \
  curl -o "${fname}" -fSL "https://github.com/rootless-containers/slirp4netns/releases/download/${SLIRP4NETNS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/slirp4netns-${SLIRP4NETNS_VERSION}" | sha256sum -c && \
  mv "${fname}" /out/bin/slirp4netns && \
  chmod +x /out/bin/slirp4netns && \
  echo "- slirp4netns: ${SLIRP4NETNS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG BYPASS4NETNS_VERSION
COPY --from=build-bypass4netns /out/${TARGETARCH:-amd64}/* /out/bin/
RUN echo "- bypass4netns: ${BYPASS4NETNS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG FUSE_OVERLAYFS_VERSION
RUN fname="fuse-overlayfs-$(cat /target_uname_m)" && \
  curl -o "${fname}" -fSL "https://github.com/containers/fuse-overlayfs/releases/download/${FUSE_OVERLAYFS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/fuse-overlayfs-${FUSE_OVERLAYFS_VERSION}" | sha256sum -c && \
  mv "${fname}" /out/bin/fuse-overlayfs && \
  chmod +x /out/bin/fuse-overlayfs && \
  echo "- fuse-overlayfs: ${FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION
RUN fname="containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION/v}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/containerd/fuse-overlayfs-snapshotter/releases/download/${CONTAINERD_FUSE_OVERLAYFS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" && \
  echo "- containerd-fuse-overlayfs: ${CONTAINERD_FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG KUBO_VERSION
COPY --from=build-kubo /out/${TARGETARCH:-amd64}/* /out/bin/
RUN echo "- Kubo (IPFS): ${KUBO_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG TINI_VERSION
RUN fname="tini-static-${TARGETARCH:-amd64}" && \
  curl -o "${fname}" -fSL "https://github.com/krallin/tini/releases/download/${TINI_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/tini-${TINI_VERSION}" | sha256sum -c && \
  cp -a "${fname}" /out/bin/tini && chmod +x /out/bin/tini && \
  echo "- Tini: ${TINI_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG BUILDG_VERSION
RUN fname="buildg-${BUILDG_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/ktock/buildg/releases/download/${BUILDG_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/buildg-${BUILDG_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" && \
  echo "- buildg: ${BUILDG_VERSION}" >> /out/share/doc/nerdctl-full/README.md

RUN echo "" >> /out/share/doc/nerdctl-full/README.md && \
  echo "## License" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/slirp4netns:    [GNU GENERAL PUBLIC LICENSE, Version 2](https://github.com/rootless-containers/slirp4netns/blob/${SLIRP4NETNS_VERSION}/COPYING)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/fuse-overlayfs: [GNU GENERAL PUBLIC LICENSE, Version 2](https://github.com/containers/fuse-overlayfs/blob/${FUSE_OVERLAYFS_VERSION}/COPYING)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/ipfs: [Combination of MIT-only license and dual MIT/Apache-2.0 license](https://github.com/ipfs/kubo/blob/${KUBO_VERSION}/LICENSE)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/{runc,bypass4netns,bypass4netnsd}: Apache License 2.0, statically linked with libseccomp ([LGPL 2.1](https://github.com/seccomp/libseccomp/blob/main/LICENSE), source code available at https://github.com/seccomp/libseccomp/)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/tini: [MIT License](https://github.com/krallin/tini/blob/${TINI_VERSION}/LICENSE)" >> /out/share/doc/nerdctl-full/README.md && \
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
  apparmor \
  bash-completion \
  ca-certificates curl \
  iproute2 iptables \
  dbus dbus-user-session systemd systemd-sysv \
  fuse3
ARG CONTAINERIZED_SYSTEMD_VERSION
RUN curl -L -o /docker-entrypoint.sh https://raw.githubusercontent.com/AkihiroSuda/containerized-systemd/${CONTAINERIZED_SYSTEMD_VERSION}/docker-entrypoint.sh && \
  chmod +x /docker-entrypoint.sh
COPY --from=out-full / /usr/local/
RUN perl -pi -e 's/multi-user.target/docker-entrypoint.target/g' /usr/local/lib/systemd/system/*.service && \
  systemctl enable containerd buildkit stargz-snapshotter && \
  mkdir -p /etc/bash_completion.d && \
  nerdctl completion bash >/etc/bash_completion.d/nerdctl && \
  mkdir -p -m 0755 /etc/cni
COPY ./Dockerfile.d/etc_containerd_config.toml /etc/containerd/config.toml
COPY ./Dockerfile.d/etc_buildkit_buildkitd.toml /etc/buildkit/buildkitd.toml
VOLUME /var/lib/containerd
VOLUME /var/lib/buildkit
VOLUME /var/lib/containerd-stargz-grpc
VOLUME /var/lib/nerdctl
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["bash", "--login", "-i"]

# convert GO_VERSION=1.16 to the latest release such as "go1.16.1"
FROM golang:${GO_VERSION}-alpine AS goversion
RUN go env GOVERSION > /GOVERSION

FROM base AS test-integration
ARG DEBIAN_FRONTEND=noninteractive
# `expect` package contains `unbuffer(1)`, which is used for emulating TTY for testing
RUN apt-get update && \
  apt-get install -qq -y \
  expect git
COPY --from=goversion /GOVERSION /GOVERSION
ARG TARGETARCH
RUN curl -L https://golang.org/dl/$(cat /GOVERSION).linux-${TARGETARCH:-amd64}.tar.gz | tar xzvC /usr/local
ENV PATH=/usr/local/go/bin:$PATH
ARG GOTESTSUM_VERSION
RUN GOBIN=/usr/local/bin go install gotest.tools/gotestsum@${GOTESTSUM_VERSION}
COPY . /go/src/github.com/containerd/nerdctl
WORKDIR /go/src/github.com/containerd/nerdctl
VOLUME /tmp
ENV CGO_ENABLED=0
# copy cosign binary for integration test
COPY --from=gcr.io/projectsigstore/cosign:v2.0.0@sha256:728944a9542a7235b4358c4ab2bcea855840e9d4b9594febca5c2207f5da7f38 /ko-app/cosign /usr/local/bin/cosign
# installing soci for integration test
ARG SOCI_SNAPSHOTTER_VERSION
RUN fname="soci-snapshotter-${SOCI_SNAPSHOTTER_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fSL "https://github.com/awslabs/soci-snapshotter/releases/download/v${SOCI_SNAPSHOTTER_VERSION}/${fname}" && \
  tar -C /usr/local/bin -xvf "${fname}" soci soci-snapshotter-grpc
# enable offline ipfs for integration test
COPY ./Dockerfile.d/test-integration-etc_containerd-stargz-grpc_config.toml /etc/containerd-stargz-grpc/config.toml
COPY ./Dockerfile.d/test-integration-ipfs-offline.service /usr/local/lib/systemd/system/
COPY ./Dockerfile.d/test-integration-buildkit-nerdctl-test.service /usr/local/lib/systemd/system/
COPY ./Dockerfile.d/test-integration-soci-snapshotter.service /usr/local/lib/systemd/system/
RUN cp /usr/local/bin/tini /usr/local/bin/tini-custom
# using test integration containerd config
COPY ./Dockerfile.d/test-integration-etc_containerd_config.toml /etc/containerd/config.toml
# install ipfs service. avoid using 5001(api)/8080(gateway) which are reserved by tests.
RUN systemctl enable test-integration-ipfs-offline test-integration-buildkit-nerdctl-test test-integration-soci-snapshotter && \
  ipfs init && \
  ipfs config Addresses.API "/ip4/127.0.0.1/tcp/5888" && \
  ipfs config Addresses.Gateway "/ip4/127.0.0.1/tcp/5889"
# install nydus components
ARG NYDUS_VERSION
RUN curl -L -o nydus-static.tgz "https://github.com/dragonflyoss/image-service/releases/download/${NYDUS_VERSION}/nydus-static-${NYDUS_VERSION}-linux-${TARGETARCH}.tgz" && \
  tar xzf nydus-static.tgz && \
  mv nydus-static/nydus-image nydus-static/nydusd nydus-static/nydusify /usr/bin/ && \
  rm nydus-static.tgz
CMD ["gotestsum", "--format=testname", "--rerun-fails=2", "--packages=github.com/containerd/nerdctl/cmd/nerdctl/...", \
  "--", "-timeout=30m", "-args", "-test.kill-daemon"]

FROM test-integration AS test-integration-rootless
# Install SSH for creating systemd user session.
# (`sudo` does not work for this purpose,
#  OTOH `machinectl shell` can create the session but does not propagate exit code)
RUN apt-get update && \
  apt-get install -qq -y \
  uidmap \
  openssh-server openssh-client
# TODO: update containerized-systemd to enable sshd by default, or allow `systemctl wants <TARGET> sshd` here
RUN ssh-keygen -q -t rsa -f /root/.ssh/id_rsa -N '' && \
  useradd -m -s /bin/bash rootless && \
  mkdir -p -m 0700 /home/rootless/.ssh && \
  cp -a /root/.ssh/id_rsa.pub /home/rootless/.ssh/authorized_keys && \
  mkdir -p /home/rootless/.local/share && \
  chown -R rootless:rootless /home/rootless
COPY ./Dockerfile.d/etc_systemd_system_user@.service.d_delegate.conf /etc/systemd/system/user@.service.d/delegate.conf
# ipfs daemon for rootless containerd will be enabled in /test-integration-rootless.sh
RUN systemctl disable test-integration-ipfs-offline
VOLUME /home/rootless/.local/share
RUN go test -o /usr/local/bin/nerdctl.test -c ./cmd/nerdctl
COPY ./Dockerfile.d/test-integration-rootless.sh /
CMD ["/test-integration-rootless.sh", \
  "gotestsum", "--format=testname", "--rerun-fails=2", "--raw-command", \
  "--", "/usr/local/go/bin/go", "tool", "test2json", "-t", "-p", "github.com/containerd/nerdctl/cmd/nerdctl",  \
  "/usr/local/bin/nerdctl.test", "-test.v", "-test.timeout=30m", "-test.kill-daemon"]

# test for CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER=slirp4netns
FROM test-integration-rootless AS test-integration-rootless-port-slirp4netns
COPY ./Dockerfile.d/home_rootless_.config_systemd_user_containerd.service.d_port-slirp4netns.conf /home/rootless/.config/systemd/user/containerd.service.d/port-slirp4netns.conf
RUN chown -R rootless:rootless /home/rootless/.config

FROM test-integration AS test-integration-ipv6
CMD ["gotestsum", "--format=testname", "--rerun-fails=2", "--packages=github.com/containerd/nerdctl/cmd/nerdctl/...", \
  "--", "-timeout=30m", "-args", "-test.kill-daemon", "-test.ipv6"]

FROM base AS demo
