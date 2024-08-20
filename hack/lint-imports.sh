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

# FIXME: goimports-reviser is currently broken when it comes to ./...
# Specifically, it will ignore arguments, and will return exit 0 regardless
# This here is a workaround, until they fix it upstream: https://github.com/incu6us/goimports-reviser/pull/157

# shellcheck disable=SC2034,SC2015
set -o errexit -o errtrace -o functrace -o nounset -o pipefail

ex=0

while read -r file; do
  goimports-reviser -list-diff -set-exit-status -output stdout -company-prefixes "github.com/containerd" "$file" || {
    ex=$?
    >&2 printf "Imports are not listed properly in %s. Consider calling make lint-fix-imports.\n" "$file"
  }
done < <(find ./ -type f -name '*.go')

exit "$ex"
