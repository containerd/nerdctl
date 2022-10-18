#!/bin/bash

#   Copyright The containerd Authors.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

# Example script to prepare swarm key secret for IPFS bootstrap,
# Example: ./bootstrap.yaml.sh > ./bootstrap.yaml

set -eu -o pipefail

for d in ipfs-key ipfs-swarm-key-gen ; do
    if ! which $d >/dev/null 2>&1 ; then
        echo "$d not found. See https://cluster.ipfs.io/documentation/guides/k8s/"
        exit 1
    fi
done

TMPIDFILE=$(mktemp)
BOOTSTRAP_KEY=$(ipfs-key 2>"${TMPIDFILE}" | base64 -w 0)
ID=$(cat "${TMPIDFILE}" | grep "ID " | sed -E 's/[^:]*: (.*)/\1/')
rm "${TMPIDFILE}"

BOOTSTRAP_PEER_PRIV_KEY=$(echo "${BOOTSTRAP_KEY}" | base64 -w 0)
CLUSTER_SECRET=$(od  -vN 32 -An -tx1 /dev/urandom | tr -d ' \n' | base64 -w 0 -)

SWARM_KEY=$(ipfs-swarm-key-gen | base64 -w 0)

cat <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: env-config
data:
  cluster-bootstrap-peer-id: $ID
---
apiVersion: v1
kind: Secret
metadata:
  name: secret-config
type: Opaque
data:
  cluster-secret: $CLUSTER_SECRET
  cluster-bootstrap-peer-priv-key: $BOOTSTRAP_PEER_PRIV_KEY
  ipfs-swarm-key: $SWARM_KEY
EOF
