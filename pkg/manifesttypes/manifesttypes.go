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

package manifesttypes

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type (

	// DockerManifestEntry represents a single manifest entry in Docker's verbose format
	DockerManifestEntry struct {
		Ref              string             `json:"Ref"`
		Descriptor       ocispec.Descriptor `json:"Descriptor"`
		Raw              string             `json:"Raw"`
		SchemaV2Manifest interface{}        `json:"SchemaV2Manifest,omitempty"`
		OCIManifest      interface{}        `json:"OCIManifest,omitempty"`
	}
	ManifestStruct struct {
		SchemaVersion int                  `json:"schemaVersion"`
		MediaType     string               `json:"mediaType"`
		Config        ocispec.Descriptor   `json:"config"`
		Layers        []ocispec.Descriptor `json:"layers"`
		Annotations   map[string]string    `json:"annotations,omitempty"`
	}

	DockerManifestStruct ManifestStruct

	DockerManifestListStruct struct {
		SchemaVersion int                  `json:"schemaVersion"`
		MediaType     string               `json:"mediaType"`
		Manifests     []ocispec.Descriptor `json:"manifests"`
	}

	OCIIndexStruct ocispec.Index

	OCIManifestStruct ManifestStruct
)
