# Example: Node-to-Node image sharing on Kubernetes using `nerdctl ipfs registry`

Usage:
- Generate `bootstrap.yaml` by executing `bootstrap.yaml.sh` (e.g. `./bootstrap.yaml.sh > ${DIR_LOCATION}/bootstrap.yaml`)
  - [`ipfs-swarm-key-gen`](https://github.com/Kubuxu/go-ipfs-swarm-key-gen) is required (see https://github.com/ipfs/kubo/blob/v0.15.0/docs/experimental-features.md#private-networks)
- Deploy `bootstrap.yaml` and `nerdctl-ipfs-registry.yaml` (e.g. using `kubectl apply`)
- Make sure nodes contain containerd >= v1.5.8

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
$ kind create cluster --image=kindest/node:v1.23.1 --config=/tmp/kindconfig.yaml
$ ./bootstrap.yaml.sh > ./bootstrap.yaml
$ kubectl apply -f .
```

Prepare `kind-worker` (1st node) for importing an image to IPFS

(in `kind-worker`)

```console
$ docker exec -it kind-worker /bin/bash
(kind-worker)# NERDCTL_VERSION=0.15.0
(kind-worker)# curl -sSL --output /tmp/nerdctl.tgz https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz
(kind-worker)# tar zxvf /tmp/nerdctl.tgz -C /usr/local/bin/
```

Add an image to `kind-worker`.

```console
$ docker exec -it kind-worker /bin/bash
(kind-worker)# mkdir -p /tmp/ipfsapi ; echo -n /ip4/127.0.0.1/tcp/5001 >  /tmp/ipfsapi/api
(kind-worker)# export IPFS_PATH=/tmp/ipfsapi
(kind-worker)# nerdctl pull ghcr.io/stargz-containers/jenkins:2.60.3-org
(kind-worker)# nerdctl push ipfs://ghcr.io/stargz-containers/jenkins:2.60.3-org
(kind-worker)# nerdctl rmi ghcr.io/stargz-containers/jenkins:2.60.3-org
```

The image added to `kind-worker` is shared to `kind-worker2` via IPFS.
You can run this image on all worker nodes using the following manifest (assuming the image is added to IPFS as CID `localhost:5050/ipfs/bafkreife3j4tgtx23jcautprornrdp2p4g3j3ndnidzdlrpd7unbpnwkce`).

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
        image: localhost:5050/ipfs/bafkreife3j4tgtx23jcautprornrdp2p4g3j3ndnidzdlrpd7unbpnwkce
        resources:
          requests:
            cpu: 1
EOF
```

The image runs on all nodes.

```console
$ kubectl get pods -owide | grep jenkins
jenkins-7bd8f96d79-2jbc6          1/1     Running   0          69s    10.244.1.3   kind-worker    <none>           <none>
jenkins-7bd8f96d79-jb5lm          1/1     Running   0          69s    10.244.2.4   kind-worker2   <none>           <none>
```

## Example Dockerfile of nerdctl

```Dockerfile
FROM ubuntu:20.04
ARG NERDCTL_VERSION=0.16.0
RUN apt-get update -y && apt-get install -y curl && \
    curl -sSL --output /tmp/nerdctl.tgz https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-${TARGETARCH:-amd64}.tar.gz && \
    tar zxvf /tmp/nerdctl.tgz -C /usr/local/bin/ && \
    rm /tmp/nerdctl.tgz
ENTRYPOINT [ "/usr/local/bin/nerdctl", "ipfs", "registry", "serve" ]
```
