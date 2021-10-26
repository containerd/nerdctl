# support custom global flags config

nerdctl provides config-file support.

## Options for `nerdctl --config-file`

support for global flags such as debug,debug-full,insecure-registry,address,namespace,snapshotter,cni-path,cni-netconfpath,data-root   cgroup-manager

for example /etc/nerdctl/nerdctl.toml

```
debug = true
insecure-registry = true
address = "/run/containerd/containerd.sock"
namespace="k8s.io"
```
will show debug log,can pull http registries and show k8s.io namespace containers
the priority is manual command value > env value > configfile value > default value
