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
# @BINARY: the binary checksums are verified via Dockerfile.d/SHA256SUMS.d/<COMPONENT>-<VERSION>
ARG CONTAINERD_VERSION=v2.3.2@fff62f14765df376e5fc36f5a8f8e795b5670f61
ARG RUNC_VERSION=v1.5.0@c4bb59526d0c9cf3a3a46a04d08ca031749a2119
ARG CNI_PLUGINS_VERSION=v1.9.1@BINARY

# Extra deps: Build
ARG BUILDKIT_VERSION=v0.31.1@BINARY
# Extra deps: Lazy-pulling
ARG STARGZ_SNAPSHOTTER_VERSION=v0.18.2@BINARY
# Extra deps: Encryption
ARG IMGCRYPT_VERSION=v2.0.2@6892f4df2405cd15acbefd1dca970f53ba38bfda
# Extra deps: Rootless
ARG ROOTLESSKIT_VERSION=v3.0.1@BINARY
# Extra deps: bypass4netns
ARG BYPASS4NETNS_VERSION=v0.4.2@aa04bd3dcc48c6dae6d7327ba219bda8fe2a4634
# Extra deps: FUSE-OverlayFS
ARG FUSE_OVERLAYFS_VERSION=v1.17@BINARY
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION=v2.1.7@BINARY
# Extra deps: Init
ARG TINI_VERSION=v0.19.0@BINARY
# Extra deps: Debug
ARG BUILDG_VERSION=v0.5.3@BINARY
# Extra deps: gomodjail
ARG GOMODJAIL_VERSION=v0.3.2@c145bb1e36fe0939c5fa0467f2477878dea8e3d9

# Test deps
# Currently, the Docker Official Images and the test deps are not pinned by the hash
ARG GO_VERSION=1.26
ARG UBUNTU_VERSION=24.04
ARG CONTAINERIZED_SYSTEMD_VERSION=v0.1.1
ARG NYDUS_VERSION=v2.4.3
ARG SOCI_SNAPSHOTTER_VERSION=0.14.1
ARG KUBO_VERSION=v0.42.0

FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.9.0@sha256:c64defb9ed5a91eacb37f96ccc3d4cd72521c4bd18d5442905b95e2226b0e707 AS xx


FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-trixie AS build-base
COPY --from=xx / /
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -qq --no-install-recommends \
  make \
  git \
  jq \
  curl \
  dpkg-dev
ARG TARGETARCH
# libbtrfs: for containerd
# libseccomp: for runc and bypass4netns
RUN xx-apt-get update -qq && xx-apt-get install -qq --no-install-recommends \
  binutils \
  gcc \
  libc6-dev \
  libbtrfs-dev \
  libseccomp-dev \
  pkg-config
RUN git config --global advice.detachedHead false
ADD hack/git-checkout-tag-with-hash.sh /usr/local/bin/
ADD hack/scripts/lib.sh /usr/local/bin/http::helper

FROM build-base AS build-containerd
ARG TARGETARCH
ARG CONTAINERD_VERSION
RUN git clone --quiet --depth 1 --branch "${CONTAINERD_VERSION%%@*}" https://github.com/containerd/containerd.git /go/src/github.com/containerd/containerd
WORKDIR /go/src/github.com/containerd/containerd
RUN git-checkout-tag-with-hash.sh ${CONTAINERD_VERSION} && \
  mkdir -p /out /out/$TARGETARCH && \
  cp -a containerd.service /out
RUN GO=xx-go make STATIC=1 && \
  cp -a bin/containerd bin/containerd-shim-runc-v2 bin/ctr /out/$TARGETARCH

FROM build-base AS build-runc
ARG RUNC_VERSION
ARG TARGETARCH
RUN git clone --quiet --depth 1 --branch "${RUNC_VERSION%%@*}" https://github.com/opencontainers/runc.git /go/src/github.com/opencontainers/runc
WORKDIR /go/src/github.com/opencontainers/runc
RUN git-checkout-tag-with-hash.sh ${RUNC_VERSION} && \
  mkdir -p /out
ENV CGO_ENABLED=1
# FIXME: avoid omitting libpathrs
RUN set -x ; GO=xx-go CC=$(xx-info)-gcc STRIP=$(xx-info)-strip make RUNC_BUILDTAGS="-libpathrs" static && \
  xx-verify --static runc && cp -v -a runc /out/runc.${TARGETARCH}

FROM build-base AS build-bypass4netns
ARG BYPASS4NETNS_VERSION
ARG TARGETARCH
RUN git clone --quiet --depth 1 --branch "${BYPASS4NETNS_VERSION%%@*}" https://github.com/rootless-containers/bypass4netns.git /go/src/github.com/rootless-containers/bypass4netns
WORKDIR /go/src/github.com/rootless-containers/bypass4netns
RUN git-checkout-tag-with-hash.sh ${BYPASS4NETNS_VERSION} && \
  mkdir -p /out/${TARGETARCH}
ENV CGO_ENABLED=1
RUN GO=xx-go make static && \
  xx-verify --static bypass4netns && cp -a bypass4netns bypass4netnsd /out/${TARGETARCH}

FROM build-base AS build-gomodjail
ARG GOMODJAIL_VERSION
ARG TARGETARCH
RUN git clone --quiet --depth 1 --branch "${GOMODJAIL_VERSION%%@*}" https://github.com/AkihiroSuda/gomodjail.git /go/src/github.com/AkihiroSuda/gomodjail
WORKDIR /go/src/github.com/AkihiroSuda/gomodjail
RUN git-checkout-tag-with-hash.sh ${GOMODJAIL_VERSION} && \
  mkdir -p /out/${TARGETARCH}
RUN GO=xx-go make STATIC=1 && \
  xx-verify --static _output/bin/gomodjail && cp -a _output/bin/gomodjail /out/${TARGETARCH}

FROM build-base AS build-kubo
ARG KUBO_VERSION
ARG TARGETARCH
RUN git clone --quiet --depth 1 --branch "${KUBO_VERSION%%@*}" https://github.com/ipfs/kubo.git /go/src/github.com/ipfs/kubo
WORKDIR /go/src/github.com/ipfs/kubo
RUN git-checkout-tag-with-hash.sh ${KUBO_VERSION} && \
  mkdir -p /out/${TARGETARCH}
ENV CGO_ENABLED=0
RUN xx-go --wrap && \
  make build && \
  xx-verify --static cmd/ipfs/ipfs && cp -a cmd/ipfs/ipfs /out/${TARGETARCH}

FROM build-base AS build-dependencies
ARG TARGETARCH
ENV GOARCH=${TARGETARCH}
COPY ./Dockerfile.d/SHA256SUMS.d/ /SHA256SUMS.d
WORKDIR /nowhere
RUN echo "${TARGETARCH:-amd64}" | sed -e s/amd64/x86_64/ -e s/arm64/aarch64/ | tee /target_uname_m
RUN mkdir -p /out/share/doc/nerdctl-full && touch /out/share/doc/nerdctl-full/README.md
ARG CONTAINERD_VERSION
COPY --from=build-containerd /out/${TARGETARCH:-amd64}/* /out/bin/
COPY --from=build-containerd /out/containerd.service /out/lib/systemd/system/containerd.service
RUN echo "- containerd: ${CONTAINERD_VERSION%%@*}" >> /out/share/doc/nerdctl-full/README.md
ARG RUNC_VERSION
COPY --from=build-runc /out/runc.${TARGETARCH:-amd64} /out/bin/runc
RUN echo "- runc: ${RUNC_VERSION%%@*}" >> /out/share/doc/nerdctl-full/README.md
ARG CNI_PLUGINS_VERSION
RUN CNI_PLUGINS_VERSION=${CNI_PLUGINS_VERSION%%@*}; \
  fname="cni-plugins-${TARGETOS:-linux}-${TARGETARCH:-amd64}-${CNI_PLUGINS_VERSION}.tgz" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/cni-plugins-${CNI_PLUGINS_VERSION}" | sha256sum -c && \
  mkdir -p /out/libexec/cni && \
  tar xzf "${fname}" -C /out/libexec/cni && \
  rm -f "${fname}" && \
  echo "- CNI plugins: ${CNI_PLUGINS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG BUILDKIT_VERSION
RUN BUILDKIT_VERSION=${BUILDKIT_VERSION%%@*}; \
  fname="buildkit-${BUILDKIT_VERSION}.${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/moby/buildkit/releases/download/${BUILDKIT_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/buildkit-${BUILDKIT_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out && \
  rm -f "${fname}" /out/bin/buildkit-qemu-* /out/bin/buildkit-cni-* /out/bin/buildkit-runc && \
  for f in /out/libexec/cni/*; do [ -x "$f" ] && [ -f "$f" ] && ln -s ../libexec/cni/$(basename $f) /out/bin/buildkit-cni-$(basename $f); done && \
  echo "- BuildKit: ${BUILDKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
# NOTE: github.com/moby/buildkit/examples/systemd is not included in BuildKit v0.8.x, will be included in v0.9.x
RUN cd /out/lib/systemd/system && \
  sedcomm='s@bin/containerd@bin/buildkitd@g; s@(Description|Documentation)=.*@@' && \
  sed -E "${sedcomm}" containerd.service > buildkit.service && \
  echo "" >> buildkit.service && \
  echo "# This file was converted from containerd.service, with \`sed -E '${sedcomm}'\`" >> buildkit.service
ARG STARGZ_SNAPSHOTTER_VERSION
RUN --mount=type=secret,id=github_token,env=GITHUB_TOKEN \
  STARGZ_SNAPSHOTTER_VERSION=${STARGZ_SNAPSHOTTER_VERSION%%@*}; \
  fname="stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/containerd/stargz-snapshotter/releases/download/${STARGZ_SNAPSHOTTER_VERSION}/${fname}" && \
  http::helper github::file containerd/stargz-snapshotter script/config/etc/systemd/system/stargz-snapshotter.service "${STARGZ_SNAPSHOTTER_VERSION}" > "stargz-snapshotter.service" && \
  grep "${fname}" "/SHA256SUMS.d/stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}" | sha256sum -c - && \
  grep "stargz-snapshotter.service" "/SHA256SUMS.d/stargz-snapshotter-${STARGZ_SNAPSHOTTER_VERSION}" | sha256sum -c - && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" /out/bin/stargz-store && \
  mv stargz-snapshotter.service /out/lib/systemd/system/stargz-snapshotter.service && \
  echo "- Stargz Snapshotter: ${STARGZ_SNAPSHOTTER_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG IMGCRYPT_VERSION
RUN git clone --quiet --depth 1 --branch "${IMGCRYPT_VERSION%%@*}" https://github.com/containerd/imgcrypt.git /go/src/github.com/containerd/imgcrypt && \
  cd /go/src/github.com/containerd/imgcrypt && \
  git-checkout-tag-with-hash.sh "${IMGCRYPT_VERSION}" && \
  CGO_ENABLED=0 make && DESTDIR=/out make install && \
  echo "- imgcrypt: ${IMGCRYPT_VERSION%%@*}" >> /out/share/doc/nerdctl-full/README.md
ARG BYPASS4NETNS_VERSION
COPY --from=build-bypass4netns /out/${TARGETARCH:-amd64}/* /out/bin/
RUN echo "- bypass4netns: ${BYPASS4NETNS_VERSION%%@*}" >> /out/share/doc/nerdctl-full/README.md
ARG FUSE_OVERLAYFS_VERSION
RUN FUSE_OVERLAYFS_VERSION=${FUSE_OVERLAYFS_VERSION%%@*}; \
  fname="fuse-overlayfs-$(cat /target_uname_m)" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/containers/fuse-overlayfs/releases/download/${FUSE_OVERLAYFS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/fuse-overlayfs-${FUSE_OVERLAYFS_VERSION}" | sha256sum -c && \
  mv "${fname}" /out/bin/fuse-overlayfs && \
  chmod +x /out/bin/fuse-overlayfs && \
  echo "- fuse-overlayfs: ${FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CONTAINERD_FUSE_OVERLAYFS_VERSION
RUN CONTAINERD_FUSE_OVERLAYFS_VERSION=${CONTAINERD_FUSE_OVERLAYFS_VERSION%%@*}; \
  fname="containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION##*v}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/containerd/fuse-overlayfs-snapshotter/releases/download/${CONTAINERD_FUSE_OVERLAYFS_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" && \
  echo "- containerd-fuse-overlayfs: ${CONTAINERD_FUSE_OVERLAYFS_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG TINI_VERSION
RUN TINI_VERSION=${TINI_VERSION%%@*}; \
  fname="tini-static-${TARGETARCH:-amd64}" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/krallin/tini/releases/download/${TINI_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/tini-${TINI_VERSION}" | sha256sum -c && \
  cp -a "${fname}" /out/bin/tini && chmod +x /out/bin/tini && \
  echo "- Tini: ${TINI_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG BUILDG_VERSION
# FIXME: this is a mildly-confusing approach. Buildkit will perform some "smart" replacement at build time and output
# confusing debugging information, eg: BUILDG_VERSION will appear as if the original ARG value was used.
RUN BUILDG_VERSION=${BUILDG_VERSION%%@*}; \
  fname="buildg-${BUILDG_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}.tar.gz" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/ktock/buildg/releases/download/${BUILDG_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/buildg-${BUILDG_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" && \
  echo "- buildg: ${BUILDG_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG ROOTLESSKIT_VERSION
RUN ROOTLESSKIT_VERSION=${ROOTLESSKIT_VERSION%%@*}; \
  fname="rootlesskit-$(cat /target_uname_m).tar.gz" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/rootless-containers/rootlesskit/releases/download/${ROOTLESSKIT_VERSION}/${fname}" && \
  grep "${fname}" "/SHA256SUMS.d/rootlesskit-${ROOTLESSKIT_VERSION}" | sha256sum -c && \
  tar xzf "${fname}" -C /out/bin && \
  rm -f "${fname}" /out/bin/rootlesskit-docker-proxy && \
  echo "- RootlessKit: ${ROOTLESSKIT_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG GOMODJAIL_VERSION
COPY --from=build-gomodjail /out/${TARGETARCH:-amd64}/* /out/bin/
RUN echo "- gomodjail: ${GOMODJAIL_VERSION}" >> /out/share/doc/nerdctl-full/README.md
ARG CONTAINERIZED_SYSTEMD_VERSION
RUN --mount=type=secret,id=github_token,env=GITHUB_TOKEN \
  http::helper github::file AkihiroSuda/containerized-systemd docker-entrypoint.sh "${CONTAINERIZED_SYSTEMD_VERSION}" > /docker-entrypoint.sh && \
  chmod +x /docker-entrypoint.sh

RUN echo "" >> /out/share/doc/nerdctl-full/README.md && \
  echo "## License" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/fuse-overlayfs: [GNU GENERAL PUBLIC LICENSE, Version 2](https://github.com/containers/fuse-overlayfs/blob/${FUSE_OVERLAYFS_VERSION%%@*}/COPYING)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/{runc,bypass4netns,bypass4netnsd}: Apache License 2.0, statically linked with libseccomp ([LGPL 2.1](https://github.com/seccomp/libseccomp/blob/main/LICENSE), source code available at https://github.com/seccomp/libseccomp/)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- bin/tini: [MIT License](https://github.com/krallin/tini/blob/${TINI_VERSION%%@*}/LICENSE)" >> /out/share/doc/nerdctl-full/README.md && \
  echo "- Other files: [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0)" >> /out/share/doc/nerdctl-full/README.md

FROM build-dependencies AS build-full
COPY . /go/src/github.com/containerd/nerdctl
RUN { echo "# nerdctl (full distribution)"; echo "- nerdctl: $(cd /go/src/github.com/containerd/nerdctl && git describe --tags)"; cat /out/share/doc/nerdctl-full/README.md; } > /out/share/doc/nerdctl-full/README.md.new; mv /out/share/doc/nerdctl-full/README.md.new /out/share/doc/nerdctl-full/README.md
WORKDIR /go/src/github.com/containerd/nerdctl
RUN BINDIR=/out/bin make binaries install
# FIXME: `gomodjail pack` depends on QEMU for non-native architecture
# TODO: gomodjail should provide a plain shell script that utilizes `zip(1)` for packing the self-extract archive, without running `gomodjail pack`..
RUN /out/bin/gomodjail pack --go-mod=/go/src/github.com/containerd/nerdctl/go.mod /out/bin/nerdctl && \
  cp -a nerdctl.gomodjail /out/bin/
COPY README.md /out/share/doc/nerdctl/
COPY docs /out/share/doc/nerdctl/docs
RUN (cd /out && find ! -type d | sort | xargs sha256sum > /tmp/SHA256SUMS ) && \
  mv /tmp/SHA256SUMS /out/share/doc/nerdctl-full/SHA256SUMS && \
  chown -R 0:0 /out

FROM scratch AS out-full
COPY --from=build-full /out /

# build-test-integration-artifacts assembles, on top of the full distribution, the additional
# binaries that are only needed to run the integration test suite (cosign, soci, ipfs, nydus).
# It is meant to be exported with `--output=type=local` and installed under /usr/local on a
# (CI) host or VM, so that the integration tests can run directly on the host with `go test`.
FROM build-full AS build-test-integration-artifacts
ARG TARGETARCH
# copy cosign binary for integration test
COPY --from=ghcr.io/sigstore/cosign/cosign:v3.0.5@sha256:be924970ba7438c22e18067dec5637946d6566eac711f5bedd1584e7137008fb /ko-app/cosign /out/bin/cosign
# installing soci for integration test
# (the static build, so that it also runs on EL hosts, whose glibc is older than what
# the default build requires)
ARG SOCI_SNAPSHOTTER_VERSION
RUN fname="soci-snapshotter-${SOCI_SNAPSHOTTER_VERSION}-${TARGETOS:-linux}-${TARGETARCH:-amd64}-static.tar.gz" && \
  curl -o "${fname}" -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/awslabs/soci-snapshotter/releases/download/v${SOCI_SNAPSHOTTER_VERSION}/${fname}" && \
  tar -C /out/bin -xvf "${fname}" soci soci-snapshotter-grpc && \
  rm -f "${fname}"
# enable offline ipfs for integration test
COPY --from=build-kubo /out/${TARGETARCH:-amd64}/* /out/bin/
# install nydus components
ARG NYDUS_VERSION
RUN curl -o nydus-static.tgz -fsSL --retry 5 --retry-delay 5 --retry-max-time 120 --connect-timeout 20 --proto '=https' --tlsv1.2 "https://github.com/dragonflyoss/image-service/releases/download/${NYDUS_VERSION}/nydus-static-${NYDUS_VERSION}-linux-${TARGETARCH}.tgz" && \
  tar xzf nydus-static.tgz && \
  mv nydus-static/nydus-image nydus-static/nydusd nydus-static/nydusify /out/bin/ && \
  rm -rf nydus-static.tgz nydus-static
# tests need a tini-custom binary
RUN cp /out/bin/tini /out/bin/tini-custom

FROM scratch AS out-test-integration-artifacts
COPY --from=build-test-integration-artifacts /out /

FROM ubuntu:${UBUNTU_VERSION} AS base
# fuse3 is required by stargz snapshotter
RUN apt-get update -qq && apt-get install -qq -y --no-install-recommends \
  apparmor \
  bash-completion \
  ca-certificates curl \
  iproute2 iptables \
  dbus dbus-user-session systemd systemd-sysv \
  fuse3
COPY --from=build-full /docker-entrypoint.sh /docker-entrypoint.sh
COPY --from=out-full / /usr/local/
RUN perl -pi -e 's/multi-user.target/docker-entrypoint.target/g' /usr/local/lib/systemd/system/*.service && \
  systemctl enable containerd buildkit stargz-snapshotter && \
  mkdir -p /etc/systemd/system/docker-entrypoint.service.d && \
  { echo "# docker-entrypoint.service runs the command passed to \`docker run\`: delay it"; \
    echo "# until the daemons are ready, so that \`docker run ... nerdctl run ...\` works"; \
    echo "[Unit]"; \
    echo "After=containerd.service buildkit.service stargz-snapshotter.service"; \
  } >/etc/systemd/system/docker-entrypoint.service.d/10-after-daemons.conf && \
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

FROM base AS demo
