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
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// IDMap contains the mappings of Uids and Gids.
//
//nolint:revive
type ContainerdIDMap struct {
	UidMap []specs.LinuxIDMapping `json:"UidMap"`
	GidMap []specs.LinuxIDMapping `json:"GidMap"`
}

// Marshal serializes the IDMap object into two strings:
// one uidmap list and another one for gidmap list
func (i *ContainerdIDMap) Marshal() (string, string) {
	marshal := func(mappings []specs.LinuxIDMapping) string {
		var arr []string
		for _, m := range mappings {
			arr = append(arr, serializeLinuxIDMapping(m))
		}
		return strings.Join(arr, ",")
	}
	return marshal(i.UidMap), marshal(i.GidMap)
}

// serializeLinuxIDMapping marshals a LinuxIDMapping object to string
func serializeLinuxIDMapping(m specs.LinuxIDMapping) string {
	return fmt.Sprintf("%d:%d:%d", m.ContainerID, m.HostID, m.Size)
}
