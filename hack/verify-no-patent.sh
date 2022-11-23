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

echo "Verifying that the patented NewARC() is NOT compiled in (https://github.com/hashicorp/golang-lru/blob/v0.5.4/arc.go#L15)"
set -eux -o pipefail

# Clear GO_BUILD_LDFLAGS to embed the symbols
GO_BUILD_LDFLAGS="" make
go tool nm ./_output/nerdctl >_output/nerdctl.sym

if ! grep -w -F main.main ./_output/nerdctl.sym; then
	echo >&2 "ERROR: the symbol file seems corrupted"
	exit 1
fi

if grep -w NewARC ./_output/nerdctl.sym; then
	echo >&2 "ERROR: patented NewARC() might be compiled in?"
	exit 1
fi

echo "OK"
