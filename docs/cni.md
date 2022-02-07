# Using CNI with nerdctl

nerdctl uses CNI plugins for its container network, you can set network by
either `--network` or `--net` option.

## Basic networks

nerdctl support some basic types of CNI plugins without any configuration
needed(you should have CNI plugin be installed), for Linux systems the basic
CNI plugin types are `bridge`, `portmap`, `firewall`, `tuning`, `loopback`, for Windows
system, the supported CNI plugin types are `nat` only.

The default network `bridge` for Linux and `nat` for Windows if you
don't set any network options.

Configuration of the default network `bridge` of Linux:

```json
{
  "cniVersion": "0.4.0",
  "name": "bridge",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "nerdctl0",
      "isGateway": true,
      "ipMasq": true,
      "hairpinMode": true,
      "ipam": {
        "type": "host-local",
        "routes": [{ "dst": "0.0.0.0/0" }],
        "ranges": [
          [
            {
              "subnet": "10.4.0.1",
              "gateway": "10.4.0.0/24"
            }
          ]
        ]
      }
    },
    {
      "type": "portmap",
      "capabilities": {
        "portMappings": true
      }
    },
    {
      "type": "firewall"
    },
    {
      "type": "tuning"
    },
    {
      "type": "loopback",
      "name": "lo"
    },
    {
      "type": "isolation"
    }
  ]
}
```

When CNI plugin `isolation` be installed, will inject isolation configuration `{"type":"isolation"}` automatically.

## Custom networks

You can also customize your CNI network by providing configuration files.
For example you have one configuration file(`/etc/cni/net.d/10-mynet.conf`)
for `bridge` network:

```json
{
  "cniVersion": "0.4.0",
  "name": "mynet",
  "type": "bridge",
  "bridge": "cni0",
  "isGateway": true,
  "ipMasq": true,
  "ipam": {
    "type": "host-local",
    "subnet": "172.19.0.0/24",
    "routes": [
      { "dst": "0.0.0.0/0" }
    ]
  }
}
```

This will configure a new CNI network with the name `mynet`, and you can use
this network to create a container:

```console
# nerdctl run -it --net mynet --rm alpine ip addr show
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host
       valid_lft forever preferred_lft forever
3: eth0@if6120: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1500 qdisc noqueue state UP
    link/ether 5e:5b:3f:0c:36:56 brd ff:ff:ff:ff:ff:ff
    inet 172.19.0.51/24 brd 172.19.0.255 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 fe80::5c5b:3fff:fe0c:3656/64 scope link tentative
       valid_lft forever preferred_lft forever
```

## Bridge Isolation Plugin

If you have the [CNI isolation plugin](https://github.com/AkihiroSuda/cni-isolation) installed, the `isolation` plugin will be used automatically.
