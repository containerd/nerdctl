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
    if [ ! -x /vagrant/_output/nerdctl.test ]; then
      echo "Run 'GOOS=linux make nerdctl.test' before running 'vagrant up'"
      exit 1
    fi

    # Install RPMs
    dnf install -y \
      make \
      containerd \
      containernetworking-plugins \
      iptables \
      slirp4netns \
      policycoreutils-python-utils

    # SELinux workaround (https://github.com/moby/moby/issues/41230)
    semanage permissive -a iptables_t

    # Install runc
    RUNC_VERSION=1.0.0-rc93
    # remove rpm version of runc, which doesn't support cgroup v2
    rm -f /usr/bin/runc
    curl -o /usr/local/sbin/runc -fsSL https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.amd64
    chmod +x /usr/local/sbin/runc

    # Install RootlessKit
    ROOTLESSKIT_VERSION=0.14.0-beta.0
    curl -sSL https://github.com/rootless-containers/rootlesskit/releases/download/v${ROOTLESSKIT_VERSION}/rootlesskit-$(uname -m).tar.gz | tar Cxzv /usr/local/bin

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
