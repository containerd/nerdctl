# Example: Node-to-Node image sharing on Kubernetes with lazy pulling using `nerdctl ipfs registry` with eStargz and Stargz Snapshotter

This directory contains an example Kubernetes setup for node-to-node image sharing with lazy pulling (eStargz).

Usage:
- Generate `bootstrap.yaml` by executing `bootstrap.yaml.sh` (e.g. `./bootstrap.yaml.sh > ${DIR_LOCATION}/bootstrap.yaml`)
  - [`ipfs-swarm-key-gen`](https://github.com/Kubuxu/go-ipfs-swarm-key-gen) is required (see https://github.com/ipfs/kubo/blob/v0.15.0/docs/experimental-features.md#private-networks)
- Deploy `bootstrap.yaml` and `nerdctl-ipfs-registry.yaml` (e.g. using `kubectl apply`)
- Make sure nodes contain containerd >= v1.5.8 and stargz-snapshotter.
  - Here we use `ghcr.io/containerd/stargz-snapshotter:0.12.1-kind` that contains both of them. (This image requires kind >= 0.16.0)
- You might want to change some configuration written in `nerdctl-ipfs-registry.yaml` (e.g. [chaning profile based on your node's resouce requirements](https://docs.ipfs.tech/how-to/default-profile/#available-profiles))

## About eStargz and Stargz Snapshotter

eStargz is an OCI-compatible image format to enable to startup a container before the entire image contents become locally available.
This allows fast cold startup of containers.
Necessary chunks of image contents are fetched to the node on-demand.
This technique is called *lazy pulling*.
[Stargz Snapshotter](https://github.com/containerd/stargz-snapshotter) is the plugin of containerd that enables lazy pulling of eStargz on containerd.

This example runs stargz snapshotter on each node as a systemd service and plugs it into containerd on the same node.
containerd on each node performs lazy pulling of eStargz images from IPFS via `nerdct ipfs registry`.
Thus, the eStargz image starts up without waiting for the entire contents becoming locally available so faster cold start can be expected.

For more details about eStargz and lazy pulling, please refer to [Stargz Snapshotter](https://github.com/containerd/stargz-snapshotter) repository.

## Example on kind

Prepare cluster (make sure kind nodes contain containerd >= v1.5.8).

```console
$ cat <<EOF > /tmp/kindconfig.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
EOF
$ kind create cluster --image=ghcr.io/containerd/stargz-snapshotter:0.12.1-kind --config=/tmp/kindconfig.yaml
$ ./bootstrap.yaml.sh > ./bootstrap.yaml
$ kubectl apply -f .
```

Prepare `kind-worker` (1st node) for importing an image to IPFS

(in `kind-worker`)

```console
$ docker exec -it kind-worker /bin/bash
(kind-worker)# NERDCTL_VERSION=0.23.0
(kind-worker)# curl -o /tmp/nerdctl.tgz -fsSL --proto '=https' --tlsv1.2 https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz
(kind-worker)# tar zxvf /tmp/nerdctl.tgz -C /usr/local/bin/
```

Add an image to `kind-worker`.

```console
$ docker exec -it kind-worker /bin/bash
(kind-worker)# mkdir -p /tmp/ipfsapi ; echo -n /ip4/127.0.0.1/tcp/5001 >  /tmp/ipfsapi/api
(kind-worker)# export IPFS_PATH=/tmp/ipfsapi
(kind-worker)# nerdctl pull ghcr.io/stargz-containers/jenkins:2.60.3-esgz
(kind-worker)# nerdctl push ipfs://ghcr.io/stargz-containers/jenkins:2.60.3-esgz
(kind-worker)# nerdctl rmi ghcr.io/stargz-containers/jenkins:2.60.3-esgz
```

> NOTE: This example copies a pre-converted eStargz image (`ghcr.io/stargz-containers/jenkins:2.60.3-esgz`) from the registry to IPFS but you can push non-eStargz image to IPFS with converting it to eStargz using `--estargz` flag of `nerdctl push`. This flag automatically performs convertion of the image to eStargz.

The eStargz image added to `kind-worker` is shared to `kind-worker2` via IPFS.
You can perform lazy pulling of this eStargz image among nodes using the following manifest.
CID of the pushed image is printed when `nerdctl push` is succeeded (we assume that the image is added to IPFS as CID `bafkreidqrxutnnuc3oilje27px5o3gggzrfyomumrprcavr7nquoy3cdje`).


```console
$ cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jenkins
spec:
  replicas: 2
  selector:
    matchLabels:
      app: jenkins
  template:
    metadata:
      labels:
        app: jenkins
    spec:
      containers:
      - name: jenkins
        image: localhost:5050/ipfs/bafkreidqrxutnnuc3oilje27px5o3gggzrfyomumrprcavr7nquoy3cdje
        resources:
          requests:
            cpu: 1
EOF
```

> NOTE1: Kubernetes doesn't support `ipfs://CID` URL on YAML as of now so we need to use `localhost:5050/ipfs/CID` form instead. In the future, this limitation should be eliminated.

> NOTE2: stargz-snapshotter currently perfoms lazy pulling via `nerdctl ipfs registry` running on localhost instead of leveraging its [native support for fetching contents via ipfs daemon](https://github.com/containerd/stargz-snapshotter/blob/v0.12.0/docs/ipfs.md). This is because of the limitation described in NOTE1 and expected to be fixed once NOTE1 is solved.

The image runs on all nodes.
You may observe faster pulling of the image by eStargz.

```console
$ kubectl get pods -owide | grep jenkins
jenkins-959bc9548-6hcwc           1/1     Running   0          8s     10.244.2.4   kind-worker    <none>           <none>
jenkins-959bc9548-rfsxm           1/1     Running   0          8s     10.244.1.3   kind-worker2   <none>           <none>
$ kubectl get pods -o name | grep jenkins | xargs -I{} kubectl describe {} | grep Pulled
  Normal  Pulled     2m13s  kubelet            Successfully pulled image "localhost:5050/ipfs/bafkreidqrxutnnuc3oilje27px5o3gggzrfyomumrprcavr7nquoy3cdje" in 1.339830318s
  Normal  Pulled     2m13s  kubelet            Successfully pulled image "localhost:5050/ipfs/bafkreidqrxutnnuc3oilje27px5o3gggzrfyomumrprcavr7nquoy3cdje" in 1.585882041s
```

eStargz filesystem is used as the rootfs of the container.
The file contents are lazily downloaded to the node.

```console
$ docker exec -it kind-worker2 mount | grep "stargz on"
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/37/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/38/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/39/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/40/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/41/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/42/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/43/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/44/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/45/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/46/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/47/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/48/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/49/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/50/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/51/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/52/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/53/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/54/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/55/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
stargz on /var/lib/containerd-stargz-grpc/snapshotter/snapshots/56/fs type fuse.rawBridge (rw,nodev,relatime,user_id=0,group_id=0,allow_other)
```
