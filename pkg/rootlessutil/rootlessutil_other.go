//go:build !linux

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

// On non-Linux platforms, the rootlessutil package always denies
// rootlessness and errors out, since RootlessKit only works on Linux
// and none of the Windows containerd runtimes can be considered rootless.
// https://github.com/containerd/nerdctl/issues/2115
package rootlessutil

import (
	"fmt"

	"github.com/rootless-containers/rootlesskit/v2/pkg/api/client"
)

// Always returns false on non-Linux platforms.
func IsRootless() bool {
	return false
}

// Always returns false on non-Linux platforms.
func IsRootlessChild() bool {
	return false
}

// Always returns false on non-Linux platforms.
func IsRootlessParent() bool {
	return false
}

// Always errors out on non-Linux platforms.
func XDGRuntimeDir() (string, error) {
	return "", fmt.Errorf("can only query XDG env vars on Linux")
}

// Always returns -1 on non-Linux platforms.
func ParentEUID() int {
	return -1
}

// Always errors out on non-Linux platforms.
func NewRootlessKitClient() (client.Client, error) {
	return nil, fmt.Errorf("cannot instantiate RootlessKit client on non-Linux hosts")
}

// Always errors out on non-Linux platforms.
func ParentMain(hostGatewayIP string) error {
	return fmt.Errorf("cannot use RootlessKit on main entry point on non-Linux hosts")
}

func RootlessContainredSockAddress() (string, error) {
	return "", fmt.Errorf("cannot inspect RootlessKit state on non-Linux hosts")
}

func DetachedNetNS() (string, error) {
	return "", nil
}

func WithDetachedNetNSIfAny(fn func() error) error {
	return fn()
}
