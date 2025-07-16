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

package testutil

import (
	_ "embed"
	"fmt"
	"sync"

	"go.yaml.in/yaml/v3"
)

//go:embed images.yaml
var rawImagesList string

var testImagesOnce sync.Once

type manifestInfo struct {
	Config    string `yaml:"config,omitempty"`
	Manifest  string `yaml:"manifest,omitempty"`
	MediaType string `yaml:"mediatype,omitempty"`
	Raw       string `yaml:"raw,omitempty"`
}

type TestImage struct {
	Ref           string                  `yaml:"ref"`
	Tag           string                  `yaml:"tag,omitempty"`
	SchemaVersion int                     `yaml:"schemaversion,omitempty"`
	MediaType     string                  `yaml:"mediatype,omitempty"`
	Digest        string                  `yaml:"digest,omitempty"`
	Variants      []string                `yaml:"variants,omitempty"`
	Manifests     map[string]manifestInfo `yaml:"manifests,omitempty"`
}

var testImages map[string]TestImage

// internal helper to lookup TestImage by key, panics if not found
func lookup(key string) TestImage {
	testImagesOnce.Do(func() {
		if err := yaml.Unmarshal([]byte(rawImagesList), &testImages); err != nil {
			fmt.Printf("Error unmarshaling test images YAML file: %v\n", err)
			panic("testing is broken")
		}
	})
	im, ok := testImages[key]
	if !ok {
		fmt.Printf("Image %s was not found in images list\n", key)
		panic("testing is broken")
	}
	return im
}

func GetTestImage(key string) string {
	im := lookup(key)
	return im.Ref + ":" + im.Tag
}

func GetTestImageWithoutTag(key string) string {
	im := lookup(key)
	return im.Ref
}

func GetTestImageConfigDigest(key, platform string) string {
	im := lookup(key)
	pd, ok := im.Manifests[platform]
	if !ok {
		panic(fmt.Sprintf("platform %s not found for image %s", platform, key))
	}
	return pd.Config
}

func GetTestImageManifestDigest(key, platform string) string {
	im := lookup(key)
	pd, ok := im.Manifests[platform]
	if !ok {
		panic(fmt.Sprintf("platform %s not found for image %s", platform, key))
	}
	return pd.Manifest
}

func GetTestImageDigest(key string) string {
	im := lookup(key)
	return im.Digest
}

func GetTestImageMediaType(key string) string {
	im := lookup(key)
	return im.MediaType
}

func GetTestImageSchemaVersion(key string) int {
	im := lookup(key)
	return im.SchemaVersion
}

func GetTestImagePlatformMediaType(key, platform string) string {
	im := lookup(key)
	pd, ok := im.Manifests[platform]
	if !ok {
		panic(fmt.Sprintf("platform %s not found for image %s", platform, key))
	}
	return pd.MediaType
}

func GetTestImageRaw(key, platform string) string {
	im := lookup(key)
	pd, ok := im.Manifests[platform]
	if !ok {
		panic(fmt.Sprintf("platform %s not found for image %s", platform, key))
	}
	return pd.Raw
}
