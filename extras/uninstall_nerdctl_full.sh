#!/bin/bash

# Check if installation path 
if [ $# -ne 1 ]; then
    echo "Usage: $0 <installation_path>"
    exit 1
fi

# Get the installed path
INSTALL_PATH="$1"

# the files and directories to be deleted
BIN_FILES=(
    "${INSTALL_PATH}/bin/buildctl"
    "${INSTALL_PATH}/bin/buildg"
    "${INSTALL_PATH}/bin/buildkit-cni-bridge"
    "${INSTALL_PATH}/bin/buildkit-cni-firewall"
    "${INSTALL_PATH}/bin/buildkit-cni-host-local"
    "${INSTALL_PATH}/bin/buildkit-cni-loopback"
    "${INSTALL_PATH}/bin/buildkitd"
    "${INSTALL_PATH}/bin/bypass4netns"
    "${INSTALL_PATH}/bin/bypass4netnsd"
    "${INSTALL_PATH}/bin/containerd"
    "${INSTALL_PATH}/bin/containerd-fuse-overlayfs-grpc"
    "${INSTALL_PATH}/bin/containerd-rootless-setuptool.sh"
    "${INSTALL_PATH}/bin/containerd-rootless.sh"
    "${INSTALL_PATH}/bin/containerd-shim-runc-v2"
    "${INSTALL_PATH}/bin/containerd-stargz-grpc"
    "${INSTALL_PATH}/bin/ctd-decoder"
    "${INSTALL_PATH}/bin/ctr"
    "${INSTALL_PATH}/bin/ctr-enc"
    "${INSTALL_PATH}/bin/ctr-remote"
    "${INSTALL_PATH}/bin/fuse-overlayfs"
    "${INSTALL_PATH}/bin/ipfs"
    "${INSTALL_PATH}/bin/nerdctl"
    "${INSTALL_PATH}/bin/rootlessctl"
    "${INSTALL_PATH}/bin/rootlesskit"
    "${INSTALL_PATH}/bin/runc"
    "${INSTALL_PATH}/bin/slirp4netns"
    "${INSTALL_PATH}/bin/tini"
)

LIB_FILES=(
    "${INSTALL_PATH}/lib/systemd/system/buildkit.service"
    "${INSTALL_PATH}/lib/systemd/system/containerd.service"
    "${INSTALL_PATH}/lib/systemd/system/stargz-snapshotter.service"
)

LIBEXEC_DIR="${INSTALL_PATH}/libexec/cni"
SHARE_DIR="${INSTALL_PATH}/share/doc/nerdctl"
SHARE_FULL_DIR="${INSTALL_PATH}/share/doc/nerdctl-full"

# stop containerd service
sudo systemctl stop containerd
sudo systemctl disable containerd

# delete /bin files
for file in "${BIN_FILES[@]}"; do
    if [ -f "$file" ]; then
        sudo rm "$file"
        echo "Deleted $file"
    fi
done

# delete /lib files
for file in "${LIB_FILES[@]}"; do
    if [ -f "$file" ]; then
        sudo rm "$file"
        echo "Deleted $file"
    fi
done

# delete libexec dir
if [ -d "$LIBEXEC_DIR" ]; then
    sudo rm -r "$LIBEXEC_DIR"
    echo "Deleted $LIBEXEC_DIR"
fi

# delete share dir
if [ -d "$SHARE_DIR" ]; then
    sudo rm -r "$SHARE_DIR"
    echo "Deleted $SHARE_DIR"
fi

if [ -d "$SHARE_FULL_DIR" ]; then
    sudo rm -r "$SHARE_FULL_DIR"
    echo "Deleted $SHARE_FULL_DIR"
fi

# reload systemd
sudo systemctl daemon-reload

echo "nerdctl-full has been uninstalled successfully."
