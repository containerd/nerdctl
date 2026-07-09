#!/bin/sh

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

# This script is intended to be usable in other projects too.
# * Must not have project-specific logics
# * Must be compatible with bash, dash, and busybox

set -eu
if [ "$#" -ne 1 ]; then
	echo "$0: checkout TAG with HASH, and validate it"
	echo "Usage: $0 TAG[@HASH]"
	exit 0
fi

: "${GIT:=git}"
TAG="$(echo "$1" | cut -d@ -f1)"
HASH=""
case "$1" in
*@*)
	HASH="$(echo "$1" | cut -d@ -f2)"
	;;
esac

"$GIT" checkout "$TAG"
HEAD="$("$GIT" rev-parse HEAD)"
if [ -z "$HASH" ]; then
	echo >&2 "WARNING: ${TAG}: commit hash was not specified (got ${HEAD})"
else
	if [ "$HEAD" != "$HASH" ]; then
		echo >&2 "ERROR: ${TAG}: expected ${HASH}, got ${HEAD}"
		exit 1
	fi
fi
