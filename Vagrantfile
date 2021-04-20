# -*- mode: ruby -*-
# vi: set ft=ruby :

# Vagrant box for testing cgroup v2
Vagrant.configure("2") do |config|
  config.vm.box = "fedora/33-cloud-base"
  memory = 4096
  cpus = 2
  config.vm.provider :virtualbox do |v|
    v.memory = memory
    v.cpus = cpus
    # The default CIDR conflicts with slirp4netns CIDR (10.0.2.0/24)
    v.customize ["modifyvm", :id, "--natnet1", "192.168.42.0/24"]
  end
  config.vm.provider :libvirt do |v|
    v.memory = memory
    v.cpus = cpus
  end
  config.vm.provision "shell", inline: <<-SHELL
    set -eux -o pipefail
    if [ ! -x /vagrant/_output/nerdctl ]; then
      echo "Run 'GOOS=linux make' before running 'vagrant up'"
      exit 1
    fi
    if [ ! -x /vagrant/nerdctl.test ]; then
      echo "Run 'GOOS=linux go test -c' before running 'vagrant up'"
      exit 1
    fi
    GOARCH=amd64

    # Install RPMs (TODO: remove fuse-overlayfs after release of Fedora 34)
    dnf install -y \
      make \
      containerd \
      containernetworking-plugins \
      iptables \
      slirp4netns \
      fuse-overlayfs \
      policycoreutils-python-utils
    systemctl enable --now containerd

    # SELinux workaround (https://github.com/moby/moby/issues/41230)
    semanage permissive -a iptables_t

    # Install runc
    RUNC_VERSION=1.0.0-rc93
    # remove rpm version of runc, which doesn't support cgroup v2 (TODO: remove this after release of Fedora 34)
    rm -f /usr/bin/runc
    curl -o /usr/local/sbin/runc -fsSL https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.${GOARCH}
    chmod +x /usr/local/sbin/runc

    # Install RootlessKit
    ROOTLESSKIT_VERSION=0.14.2
    curl -sSL https://github.com/rootless-containers/rootlesskit/releases/download/v${ROOTLESSKIT_VERSION}/rootlesskit-$(uname -m).tar.gz | tar Cxzv /usr/local/bin

    # Install containerd-fuse-overlayfs (TODO: remove this after release of Fedora 34)
    CONTAINERD_FUSE_OVERLAYFS_VERSION=1.0.2
    curl -sSL https://github.com/containerd/fuse-overlayfs-snapshotter/releases/download/v${CONTAINERD_FUSE_OVERLAYFS_VERSION}/containerd-fuse-overlayfs-${CONTAINERD_FUSE_OVERLAYFS_VERSION}-linux-${GOARCH}.tar.gz | tar Cxzv /usr/local/bin
    mkdir -p /home/vagrant/.config/containerd
    cat <<EOF >/home/vagrant/.config/containerd/config.toml
[proxy_plugins]
  [proxy_plugins."fuse-overlayfs"]
    type = "snapshot"
    address = "/run/user/$(id -u vagrant)/containerd-fuse-overlayfs.sock"
EOF
    chown -R vagrant /home/vagrant/.config

    # Delegate cgroup v2 controllers
    mkdir -p /etc/systemd/system/user@.service.d
    cat <<EOF >/etc/systemd/system/user@.service.d/delegate.conf
[Service]
Delegate=yes
EOF
    systemctl daemon-reload

    # Install nerdctl
    # The binary is built outside Vagrant.
    make -C /vagrant install
  SHELL
end
