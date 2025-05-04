#!/usr/bin/env bash

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

# shellcheck disable=SC2034,SC2015
set -o errexit -o errtrace -o functrace -o nounset -o pipefail

# Extract the list of domains from image names in the environment
extract::domains(){
  local fd="$1"

  local image
  local domain
  local ignore

  while read -r image; do
    IFS=" " read -r domain ignore <<<"$image"
    printf "%s\n" "$domain"
  done < <(extract::images "$1") | sort | uniq
}

# Extract the list of images from the file.
extract::images(){
  local fd="$1"
  local image
  local ref
  local domain
  local owner
  local name
  local tag
  local digest

  while read -r image; do
    [ "$image" != "" ] && [ "${image:0:1}" != "#" ] || continue
    image="${image#*=}"
    digest="${image#*@}"
    tag="${image#*:}"
    tag="${tag%@*}"
    ref="${image%%:*}"

    IFS=/ read -r domain owner name <<<"$ref"
    [ "$owner" != "" ] || {
      name="$domain"
      owner=library
      domain=docker.io
    }
    [ "$name" != "" ] || {
      name="$owner"
      owner="$domain"
      domain=docker.io
    }

    printf "%s %s %s %s\n" "$domain" "$owner/$name" "$tag" "$digest"
  done <"$fd" | sort | uniq
}

"extract::$1" "$2"
