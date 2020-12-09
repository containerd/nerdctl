# Lazy-pulling using Stargz Snapshotter

See https://github.com/containerd/stargz-snapshotter to learn what are lazy-pulling and stargz.

## Enable lazy-pulling for `nerdctl run`
- Install Stargz plugin (`containerd-stargz-grpc`) from https://github.com/containerd/stargz-snapshotter 

- Add the following to `/etc/containerd/config.toml`:
```toml
[proxy_plugins]
  [proxy_plugins.stargz]                                                                                  
    type = "snapshot"
    address = "/run/containerd-stargz-grpc/containerd-stargz-grpc.sock"
```

- Launch `containerd` and `containerd-stargz-grpc`

- Run `nerdctl` with `--snapshotter=stargz`
```console
# nerdctl --snapshotter=stargz run -it --rm ghcr.io/stargz-containers/fedora:30-esgz
```

For the list of pre-converted Stargz images, see https://github.com/containerd/stargz-snapshotter/blob/master/docs/pre-converted-images.md

### Benchmark result (Dec 9, 2020)
For running `python3 -c print("hi")`, eStargz with Stargz Snapshotter is 3-4 times faster than the legacy OCI with overlayfs snapshotter.

Legacy OCI with overlayfs snapshotter:
```console
# time nerdctl --snapshotter=overlayfs run -it --rm ghcr.io/stargz-containers/python:3.7-org python3 -c 'print("hi")'
ghcr.io/stargz-containers/python:3.7-org:                                         resolved       |++++++++++++++++++++++++++++++++++++++|
index-sha256:6008006c63b0a6043a11ac151cee572e0c8676b4ba3130ff23deff5f5d711237:    done           |++++++++++++++++++++++++++++++++++++++|
manifest-sha256:48eafda05f80010a6677294473d51a530e8f15375b6447195b6fb04dc2a30ce7: done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:f860607a6cd9751ac8db2f33cbc3ce1777a44eb3c04853e116763441a304fbf6:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:96b2c1e36db5f5910f58da2ca4f9311b0690810c7107fb055ee1541498b5061f:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:c495e8de12d26c9843a7a2bf8c68de1e5652e66d80d9bc869279f9af6f86736a:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:33382189822a108b249cf3ccd234d04c3a8dfe7d593df19c751dcfab3675d5f2:    done           |++++++++++++++++++++++++++++++++++++++|
config-sha256:94c9a318e47ab8a318582e2712bb495f92f17a7c1e50f13cc8a3e362c1b09290:   done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:6eaa0b6b8562fb4a02e140ae53b3910fc4d0db6e68660390eaef993f42e21102:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:adbdcbacafe93bf0791e49c8d3689bb78d9e60d02d384d4e14433aedae39f52c:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:756975cb9c7e7933d824af9319b512dd72a50894232761d06ef3be59981df838:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:d77915b4e630d47296770ce4cf481894885978072432456615172af463433cc5:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:5f37a0a41b6b03489dd7de0aa2a79e369fd8b219bbc36b52f3f9790dc128e74b:    done           |++++++++++++++++++++++++++++++++++++++|
elapsed: 41.9s                                                                    total:  321.3  (7.7 MiB/s)                                       
hi                                                                                                        
                                                     
real    0m51.754s                                                                                         
user    0m2.687s
sys     0m5.533s 
```

eStargz with Stargz Snapshotter:
```console
# time nerdctl --snapshotter=stargz run -it --rm ghcr.io/stargz-containers/python:3.7-esgz python3 -c 'print("hi")'
fetching sha256:2ea0dd96... application/vnd.oci.image.index.v1+json
fetching sha256:9612ff73... application/vnd.docker.distribution.manifest.v2+json
fetching sha256:34e5920e... application/vnd.docker.container.image.v1+json
hi

real    0m13.589s
user    0m0.132s
sys     0m0.158s
```

## Enable lazy-pulling for pulling base images during `nerdctl build`

- Install Stargz plugin (`containerd-stargz-grpc`) from https://github.com/containerd/stargz-snapshotter 
- Launch `buildkitd` with `--oci-worker-snapshotter=stargz` (or `--containerd-worker-snapshotter=stargz` if you use containerd worker)
- Launch `nerdctl build`. No need to specify `--snapshotter` for `nerdctl`.

## Buildint stargz images using `nerdctl build`
Unsupported yet
