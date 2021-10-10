#!/bin/sh

#   Copyright The BuildKit Authors.
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

# buildkitd-rootless.sh executes buildkitd in rootless mode.
#
# Usage: buildkitd-rootless.sh [BUILDKITD_OPTIONS]
#
# External dependencies:
# * newuidmap and newgidmap needs to be installed.
# * /etc/subuid and /etc/subgid needs to be configured for the current user.
# * RootlessKit (>= v0.10.0) needs to be installed. RootlessKit >= v0.14.1 is recommended.
# * slirp4netns (>= v0.4.0) needs to be installed. slirp4netns >= v1.1.7 is recommended.

set -e
if ! [ -w $XDG_RUNTIME_DIR ]; then
	echo "XDG_RUNTIME_DIR needs to be set and writable"
	exit 1
fi
exec rootlesskit --net=slirp4netns --copy-up=/etc --disable-host-loopback buildkitd $@
