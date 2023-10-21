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

package container

import (
	"fmt"
	"path/filepath"

	"github.com/containerd/containerd/oci"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/rootless-containers/rootlesskit/v2/pkg/child"
)

func newContainerDetachNetNs(stateDir, id string, opts *[]oci.SpecOpts) error {
	return ns.WithNetNSPath(filepath.Join(stateDir, "netns"), func(_ ns.NetNS) error {
		containerDetachNetNs := filepath.Join(stateDir, fmt.Sprintf("netns-%s", id))
		if err := child.NewNetNsWithPathWithoutEnter(containerDetachNetNs); err != nil {
			return err
		}
		*opts = append(*opts, oci.WithLinuxNamespace(specs.LinuxNamespace{
			Type: specs.NetworkNamespace,
			Path: containerDetachNetNs,
		}))
		return nil
	})
}
