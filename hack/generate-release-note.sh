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

minimal_amd64tgz="$(find _output -name '*linux-amd64.tar.gz*' -and ! -name '*full*')"
full_amd64tgz="$(find _output -name '*linux-amd64.tar.gz*' -and -name '*full*')"

minimal_amd64tgz_basename="$(basename ${minimal_amd64tgz})"
full_amd64tgz_basename="$(basename ${full_amd64tgz})"

cat <<-EOX
## Changes
(To be documented)

## Compatible containerd versions
This release of nerdctl is expected to be used with containerd v1.6 or v1.7.

## About the binaries
- Minimal (\`${minimal_amd64tgz_basename}\`): nerdctl only
- Full (\`${full_amd64tgz_basename}\`):    Includes dependencies such as containerd, runc, and CNI

### Minimal
Extract the archive to a path like \`/usr/local/bin\` or \`~/bin\` .
<details><summary>tar Cxzvvf /usr/local/bin ${minimal_amd64tgz_basename}</summary>
<p>

\`\`\`
$(tar tzvf ${minimal_amd64tgz})
\`\`\`
</p>
</details>

### Full
Extract the archive to a path like \`/usr/local\` or \`~/.local\` .

<details><summary>tar Cxzvvf /usr/local ${full_amd64tgz_basename}</summary>
<p>

\`\`\`
$(tar tzvf ${full_amd64tgz})
\`\`\`
</p>
</details>

<details><summary>Included components</summary>
<p>

See \`share/doc/nerdctl-full/README.md\`:
\`\`\`markdown
$(tar xOzf ${full_amd64tgz} share/doc/nerdctl-full/README.md)
\`\`\`
</p>
</details>

## Quick start
### Rootful
\`\`\`console
$ sudo systemctl enable --now containerd
$ sudo nerdctl run -d --name nginx -p 80:80 nginx:alpine
\`\`\`

### Rootless
\`\`\`console
$ containerd-rootless-setuptool.sh install
$ nerdctl run -d --name nginx -p 8080:80 nginx:alpine
\`\`\`

Enabling cgroup v2 is highly recommended for rootless mode, see https://rootlesscontaine.rs/getting-started/common/cgroup2/ .
EOX
