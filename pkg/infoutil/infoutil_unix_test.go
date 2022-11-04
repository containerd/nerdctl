//go:build freebsd || linux

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package infoutil

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestDistroNameAlpine(t *testing.T) {
	const etcOSRelease = `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.13.2
PRETTY_NAME="Alpine Linux v3.13"
HOME_URL="https://alpinelinux.org/"
BUG_REPORT_URL="https://bugs.alpinelinux.org/"
`
	r := strings.NewReader(etcOSRelease)
	assert.Equal(t, "Alpine Linux v3.13", distroName(r))
}

func TestDistroNameAlpineAltered(t *testing.T) {
	const etcOSRelease = `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.13.2
PRETTY_NAME=Alpine Linux v3.13 # some comment
HOME_URL="https://alpinelinux.org/"
BUG_REPORT_URL="https://bugs.alpinelinux.org/"
`
	r := strings.NewReader(etcOSRelease)
	assert.Equal(t, "Alpine Linux v3.13", distroName(r))
}

func TestDistroNameUbuntu(t *testing.T) {
	const etcOSRelease = `NAME="Ubuntu"
VERSION="20.10 (Groovy Gorilla)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 20.10"
VERSION_ID="20.10"
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
BUG_REPORT_URL="https://bugs.launchpad.net/ubuntu/"
PRIVACY_POLICY_URL="https://www.ubuntu.com/legal/terms-and-policies/privacy-policy"
VERSION_CODENAME=groovy
UBUNTU_CODENAME=groovy`
	r := strings.NewReader(etcOSRelease)
	assert.Equal(t, "Ubuntu 20.10", distroName(r))
}

func TestDistroNameCentOS(t *testing.T) {
	const etcOSRelease = `NAME="CentOS Linux"
VERSION="7 (Core)"
ID="centos"
ID_LIKE="rhel fedora"
VERSION_ID="7"
PRETTY_NAME="CentOS Linux 7 (Core)"
ANSI_COLOR="0;31"
CPE_NAME="cpe:/o:centos:centos:7"
HOME_URL="https://www.centos.org/"
BUG_REPORT_URL="https://bugs.centos.org/"

CENTOS_MANTISBT_PROJECT="CentOS-7"
CENTOS_MANTISBT_PROJECT_VERSION="7"
REDHAT_SUPPORT_PRODUCT="centos"
REDHAT_SUPPORT_PRODUCT_VERSION="7"
`
	r := strings.NewReader(etcOSRelease)
	assert.Equal(t, "CentOS Linux 7 (Core)", distroName(r))
}

func TestDistroNameEmpty(t *testing.T) {
	r := strings.NewReader("")
	assert.Equal(t, UnameO, distroName(r))
}

func TestDistroNameNoPrettyName(t *testing.T) {
	const etcOSRelease = `NAME="Foo"
VERSION = "42.0"
`
	r := strings.NewReader(etcOSRelease)
	assert.Equal(t, "Foo 42.0", distroName(r))
}
