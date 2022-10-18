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

for d in ipfs-swarm-key-gen ; do
    if ! command -v $d >/dev/null 2>&1 ; then
        echo "$d not found"
        exit 1
    fi
done

SWARM_KEY=$(ipfs-swarm-key-gen | base64 | tr -d '\n')

cat <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: secret-config
type: Opaque
data:
  ipfs-swarm-key: $SWARM_KEY
EOF
