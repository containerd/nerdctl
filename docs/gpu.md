# Using GPUs inside containers

| :zap: Requirement | nerdctl >= 0.9 |
|-------------------|----------------|

nerdctl provides docker-compatible NVIDIA GPU support.

## Prerequisites

- NVIDIA Drivers
  - Same requirement as when you use GPUs on Docker. For details, please refer to [the doc by NVIDIA](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html#pre-requisites).
- `nvidia-container-cli`
  - containerd relies on this CLI for setting up GPUs inside container. You can install this via [`libnvidia-container` package](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/arch-overview.html#libnvidia-container).

## Options for `nerdctl run --gpus`

`nerdctl run --gpus` is compatible to [`docker run --gpus`](https://docs.docker.com/engine/reference/commandline/run/#access-an-nvidia-gpu).

You can specify number of GPUs to use via `--gpus` option.
The following example exposes all available GPUs.

```
nerdctl run -it --rm --gpus all nvidia/cuda:12.3.1-base-ubuntu20.04 nvidia-smi
```

You can also pass detailed configuration to `--gpus` option as a list of key-value pairs. The following options are provided.

- `count`: number of GPUs to use. `all` exposes all available GPUs.
- `device`: IDs of GPUs to use. UUID or numbers of GPUs can be specified.
- `capabilities`: [Driver capabilities](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/user-guide.html#driver-capabilities). If unset, use default driver `utility`, `compute`.

The following example exposes a specific GPU to the container.

```
nerdctl run -it --rm --gpus '"capabilities=utility,compute",device=GPU-3a23c669-1f69-c64e-cf85-44e9b07e7a2a' nvidia/cuda:12.3.1-base-ubuntu20.04 nvidia-smi
```

## Fields for `nerdctl compose`

`nerdctl compose` also supports GPUs following [compose-spec](https://github.com/compose-spec/compose-spec/blob/master/deploy.md#devices).

You can use GPUs on compose when you specify some of the following `capabilities` in `services.demo.deploy.resources.reservations.devices`.

- `gpu`
- `nvidia`
- all allowed capabilities for `nerdctl run --gpus`

Available fields are the same as `nerdctl run --gpus`.

The following exposes all available GPUs to the container.

```
version: "3.8"
services:
  demo:
    image: nvidia/cuda:12.3.1-base-ubuntu20.04
    command: nvidia-smi
    deploy:
      resources:
        reservations:
          devices:
          - capabilities: ["utility"]
            count: all
```

## Trouble Shooting

### `nerdctl run --gpus` fails when using the Nvidia gpu-operator

If the Nvidia driver is installed by the [gpu-operator](https://github.com/NVIDIA/gpu-operator).The `nerdctl run` will fail with the error message `(FATA[0000] exec: "nvidia-container-cli": executable file not found in $PATH)`.

So, the `nvidia-container-cli` needs to be added to the PATH environment variable.

You can do this by adding the following line to your $HOME/.profile or /etc/profile (for a system-wide installation):
```
export PATH=$PATH:/usr/local/nvidia/toolkit
```

The shared libraries also need to be added to the system.
```
echo "/run/nvidia/driver/usr/lib/x86_64-linux-gnu" > /etc/ld.so.conf.d/nvidia.conf
ldconfig
```

And then, the `nerdctl run --gpus` can run successfully. eg:
```
nerdctl run -it --rm --runtime=/usr/local/nvidia/toolkit/nvidia-container-runtime --gpus all nvidia/cuda:12.3.1-base-ubuntu20.04 nvidia-smi
```
