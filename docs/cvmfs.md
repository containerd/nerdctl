# Lazy-pulling using CernVM-FS Snapshotter

CernVM-FS Snapshotter is a containerd snapshotter plugin. It is a specialized component responsible for assembling
all the layers of container images into a stacked file system that containerd can use. The snapshotter takes as input the list
of required layers and outputs a directory containing the final file system. It is also responsible to clean up the output
directory when containers using it are stopped.

See the official [documentation](https://cvmfs.readthedocs.io/en/latest/cpt-containers.html#how-to-use-the-cernvm-fs-snapshotter) to learn further information.

## Prerequisites

- Install containerd remote snapshotter plugin (`cvmfs-snapshotter`) from [here](https://github.com/cvmfs/cvmfs/tree/devel/snapshotter).

- Add the following to `/etc/containerd/config.toml`:
```toml
# Ask containerd to use this particular snapshotter
[plugins."io.containerd.grpc.v1.cri".containerd]
    snapshotter = "cvmfs-snapshotter"
    disable_snapshot_annotations = false

# Set the communication endpoint between containerd and the snapshotter
[proxy_plugins]
    [proxy_plugins.cvmfs]
        type = "snapshot"
        address = "/run/containerd-cvmfs-grpc/containerd-cvmfs-grpc.sock"
```
- The default CernVM-FS repository hosting the flat root filesystems of the container images is `unpacked.cern.ch`.
  The container images are unpacked into the CernVM-FS repository by the [DUCC](https://cvmfs.readthedocs.io/en/latest/cpt-ducc.html)
  (Daemon that Unpacks Container Images into CernVM-FS) tool.
  You can change the repository adding the following line to `/etc/containerd-cvmfs-grpc/config.toml`:
```toml
repository = "myrepo.mydomain"
```
- Launch `containerd` and `cvmfs-snapshotter`:
```console
$ systemctl start containerd cvmfs-snapshotter
```

## Enable CernVM-FS Snapshotter for `nerdctl run` and `nerdctl pull`

| :zap: Requirement | nerdctl >= 1.6.3 |
| ----------------- | ---------------- |

- Run `nerdctl` with `--snapshotter cvmfs-snapshotter` as in the example below:
```console
$ nerdctl run -it --rm --snapshotter cvmfs-snapshotter clelange/cms-higgs-4l-full:latest
```

- You can also only pull the image with CernVM-FS Snapshotter without running the container:
```console
$ nerdctl pull --snapshotter cvmfs-snapshotter clelange/cms-higgs-4l-full:latest
```

The speedup for pulling this 9 GB (4.3 GB compressed) image is shown below:
- #### with the snapshotter:
```console
$ nerdctl --snapshotter cvmfs-snapshotter pull clelange/cms-higgs-4l-full:latest
docker.io/clelange/cms-higgs-4l-full:latest:                                      resolved       |++++++++++++++++++++++++++++++++++++++|
manifest-sha256:b8acbe80629dd28d213c03cf1ffd3d46d39e573f54215a281fabce7494b3d546: done           |++++++++++++++++++++++++++++++++++++++|
config-sha256:89ef54b6c4fbbedeeeb29b1df2b9916b6d157c87cf1878ea882bff86a3093b5c:   done           |++++++++++++++++++++++++++++++++++++++|
elapsed: 4.7 s                                                                    total:  19.8 K (4.2 KiB/s)

$ nerdctl images
REPOSITORY                    TAG       IMAGE ID        CREATED           PLATFORM       SIZE     BLOB SIZE
clelange/cms-higgs-4l-full    latest    b8acbe80629d    20 seconds ago    linux/amd64    0.0 B    4.3 GiB
```
- #### without the snapshotter:
```console
$ nerdctl pull clelange/cms-higgs-4l-full:latest
docker.io/clelange/cms-higgs-4l-full:latest:                                      resolved       |++++++++++++++++++++++++++++++++++++++|
manifest-sha256:b8acbe80629dd28d213c03cf1ffd3d46d39e573f54215a281fabce7494b3d546: exists         |++++++++++++++++++++++++++++++++++++++|
config-sha256:89ef54b6c4fbbedeeeb29b1df2b9916b6d157c87cf1878ea882bff86a3093b5c:   exists         |++++++++++++++++++++++++++++++++++++++|
layer-sha256:e8114d4b0d10b33aaaa4fbc3c6da22bbbcf6f0ef0291170837e7c8092b73840a:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:a3eda0944a81e87c7a44b117b1c2e707bc8d18e9b7b478e21698c11ce3e8b819:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:8f3160776e8e8736ea9e3f6c870d14cd104143824bbcabe78697315daca0b9ad:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:22a5c05baa9db0aa7bba56ffdb2dd21246b9cf3ce938fc6d7bf20e92a067060e:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:bfcf9d498f92b72426c9d5b73663504d87249d6783c6b58d71fbafc275349ab9:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:0563e1549926b9c8beac62407bc6a420fa35bcf6f9844e5d8beeb9165325a872:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:6fff5fd7fb4eeb79a1399d9508614a84191d05e53f094832062d689245599640:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:25c39bfa66e1157415236703abc512d06cc1db31bd00fe8c3030c6d6d249dc4e:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:3cc0a0eb55eb3fb7ef0760c6bf1e567dfc56933ba5f11b5415f89228af751b72:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:a8850244786303e508b94bb31c8569310765e678c9c73bf1199310729209b803:    done           |++++++++++++++++++++++++++++++++++++++|
layer-sha256:32cdf5fc12485ac061347eb8b5c3b4a28505ce8564a7f3f83ac4241f03911176:    done           |++++++++++++++++++++++++++++++++++++++|
elapsed: 181.8s                                                                   total:  4.3 Gi (24.2 MiB/s)

$ nerdctl images
REPOSITORY                    TAG       IMAGE ID        CREATED          PLATFORM       SIZE       BLOB SIZE
clelange/cms-higgs-4l-full    latest    b8acbe80629d    4 minutes ago    linux/amd64    9.0 GiB    4.3 GiB
```
