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

package native

import (
	"encoding/json"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type ManifestEntry struct {
	ManifestDesc *ocispec.Descriptor `json:"ManifestDesc,omitempty"`
	Manifest     *ocispec.Manifest   `json:"Manifest,omitempty"`
}

type Manifest struct {
	ImageName string              `json:"ImageName,omitempty"`
	IndexDesc *ocispec.Descriptor `json:"IndexDesc,omitempty"`
	Index     *ocispec.Index      `json:"Index,omitempty"`
	Manifests []ManifestEntry     `json:"Manifests,omitempty"`
}

// MarshalJSON implements custom JSON marshaling to flatten single manifest
func (m *Manifest) MarshalJSON() ([]byte, error) {
	type ManifestAlias Manifest

	// If there's only one manifest and no index, flatten the structure
	if len(m.Manifests) == 1 && m.Index == nil && m.IndexDesc == nil {
		manifest := m.Manifests[0]
		// Check if we have either ManifestDesc or Manifest
		if manifest.ManifestDesc != nil || manifest.Manifest != nil {
			result := make(map[string]interface{})
			if manifest.ManifestDesc != nil {
				result["ManifestDesc"] = manifest.ManifestDesc
			}
			if manifest.Manifest != nil {
				result["Manifest"] = manifest.Manifest
			}
			// Return the manifest directly without the Manifests wrapper
			return json.Marshal(result)
		}
	}

	// Otherwise, use the default marshaling
	return json.Marshal((*ManifestAlias)(m))
}
