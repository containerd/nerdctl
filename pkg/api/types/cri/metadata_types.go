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

// Forked from https://github.com/containerd/containerd/blob/main/pkg/cri/store/container/metadata.go
// Copyright The containerd Authors.
// Licensed under the Apache License, Version 2.0
// NOTE: we just want to get the key information of metadata( such as logpath), not all the metadata.

package cri

import (
	"encoding/json"
	"fmt"
)

// NOTE(random-liu):
// 1) Metadata is immutable after created.
// 2) Metadata is checkpointed as containerd container label.

// metadataVersion is current version of container metadata.
const metadataVersion = "v1" // nolint

// ContainerVersionedMetadata is the internal versioned container metadata.
// nolint
type ContainerVersionedMetadata struct {
	// Version indicates the version of the versioned container metadata.
	Version string
	// Metadata's type is criContainerMetadataInternal. If not there will be a recursive call in MarshalJSON.
	Metadata criContainerMetadataInternal
}

// criContainerMetadataInternal is for internal use.
type criContainerMetadataInternal ContainerMetadata

// ContainerMetadata is the unversioned container metadata.
type ContainerMetadata struct {
	// LogPath is the container log path.
	LogPath string
}

// MarshalJSON encodes Metadata into bytes in json format.
func (c *ContainerMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(&ContainerVersionedMetadata{
		Version:  metadataVersion,
		Metadata: criContainerMetadataInternal(*c),
	})
}

// UnmarshalJSON decodes Metadata from bytes.
func (c *ContainerMetadata) UnmarshalJSON(data []byte) error {
	versioned := &ContainerVersionedMetadata{}
	if err := json.Unmarshal(data, versioned); err != nil {
		return err
	}
	// Handle old version after upgrade.
	switch versioned.Version {
	case metadataVersion:
		*c = ContainerMetadata(versioned.Metadata)
		return nil
	}
	return fmt.Errorf("unsupported version: %q", versioned.Version)
}
