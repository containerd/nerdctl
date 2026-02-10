# Using GPUs inside containers

| :zap: Requirement | nerdctl >= 0.9 |
|-------------------|----------------|

> [!NOTE]
> The description in this section applies to nerdctl v2.3 or later.
> Users of prior releases of nerdctl should refer to <https://github.com/containerd/nerdctl/blob/v2.2.0/docs/gpu.md>

nerdctl provides docker-compatible NVIDIA and AMD GPU support.

## Prerequisites

- GPU Drivers
  - Same requirement as when you use GPUs on Docker. For details, please refer to these docs by [NVIDIA](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html#pre-requisites) and [AMD](https://instinct.docs.amd.com/projects/container-toolkit/en/latest/container-runtime/quick-start-guide.html#step-2-install-the-amdgpu-driver).
- Container Toolkit
  - containerd relies on vendor Container Toolkits to make GPUs available to the containers. You can install those by following the official installation instructions from [NVIDIA](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html) and [AMD](https://instinct.docs.amd.com/projects/container-toolkit/en/latest/container-runtime/quick-start-guide.html).
- CDI Specification
  - Container Device Interface (CDI) specification for the GPU devices is required for the GPU support to work. Follow the official documentation from [NVIDIA](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/cdi-support.html) and [AMD](https://instinct.docs.amd.com/projects/container-toolkit/en/latest/container-runtime/cdi-guide.html) to ensure that the required CDI specifications are present on the system.

## Options for `nerdctl run --gpus`

`nerdctl run --gpus` is compatible to [`docker run --gpus`](https://docs.docker.com/engine/reference/commandline/run/#access-an-nvidia-gpu).

You can specify number of GPUs to use via `--gpus` option.
The following examples expose all available GPUs to the container.

```
nerdctl run -it --rm --gpus all nvidia/cuda:12.3.1-base-ubuntu20.04 nvidia-smi
```

or

```
nerdctl run -it --rm --gpus=all rocm/rocm-terminal rocm-smi
```

You can also pass detailed configuration to `--gpus` option as a list of key-value pairs. The following options are provided.

- `count`: number of GPUs to use. `all` exposes all available GPUs.
- `device`: IDs of GPUs to use. UUID or numbers of GPUs can be specified. This only works for NVIDIA GPUs.

The following example exposes a specific NVIDIA GPU to the container.

```
nerdctl run -it --rm --gpus 'device=GPU-3a23c669-1f69-c64e-cf85-44e9b07e7a2a' nvidia/cuda:12.3.1-base-ubuntu20.04 nvidia-smi
```

Note that although `capabilities` options may be provided, these are ignored when processing the GPU request since nerdctl v2.3.

## Fields for `nerdctl compose`

`nerdctl compose` also supports GPUs following [compose-spec](https://github.com/compose-spec/compose-spec/blob/master/deploy.md#devices).

You can use GPUs on compose when you specify the `driver` as `nvidia` or one or
more of the following `capabilities` in `services.demo.deploy.resources.reservations.devices`.

- `gpu`
- `nvidia`

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
          - driver: nvidia
            count: all
```

## Trouble Shooting

### `nerdctl run --gpus` fails due to an unresolvable CDI device

If the required CDI specifications for your GPU devices are not available on the
system, the `nerdctl run` command will fail with an error similar to: `CDI device injection failed: unresolvable CDI devices nvidia.com/gpu=all` (the
exact error message will depend on the vendor and the device(s) requested).

This should be the same error message that is reported when the `--device` flag
is used to request a CDI device:
```
nerdctl run --device=nvidia.com/gpu=all
```

Ensure that the NVIDIA (or AMD) Container Toolkit is installed and the requested CDI devices are present in the ouptut of `nvidia-ctk cdi list` (or `amd-ctk cdi list` for AMD GPUs):

```
$ nvidia-ctk cdi list
INFO[0000] Found 3 CDI devices
nvidia.com/gpu=0
nvidia.com/gpu=GPU-3eb87630-93d5-b2b6-b8ff-9b359caf4ee2
nvidia.com/gpu=all
```

For NVIDIA Container Toolkit, version >= v1.18.0 is recommended. See the NVIDIA Container Toolkit [CDI documentation](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/cdi-support.html) for more information.

For AMD Container Toolkit, version >= v1.2.0 is recommended. See the AMD Container Toolkit [CDI documentation](https://instinct.docs.amd.com/projects/container-toolkit/en/latest/container-runtime/cdi-guide.html) for more information.


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

And then, the `nerdctl run --gpus` can run successfully.
