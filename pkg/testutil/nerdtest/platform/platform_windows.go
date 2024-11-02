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

package platform

import (
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func DataHome() (string, error) {
	panic("not supported")
}

var (
	RegistryImageStable = "dubogus/win-registry"
	// Temporary deviations just so we do not fail on download - we need these images though
	RegistryImageNext = testutil.CommonImage
	KuboImage         = testutil.CommonImage // mirrorOf("ipfs/kubo:v0.16.0")
	DockerAuthImage   = testutil.CommonImage // mirrorOf("cesanta/docker_auth:1.7")
)
