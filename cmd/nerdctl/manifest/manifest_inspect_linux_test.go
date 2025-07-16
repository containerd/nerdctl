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

package manifest

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestManifestInspect(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", "--platform=linux/amd64", testutil.AlpineImage)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", "-f", testutil.AlpineImage)
		helpers.Anyhow("rmi", "-f", testutil.GetImageWithoutTag("alpine")+"@"+testutil.GetTestImageManifestDigest("alpine", "linux/amd64"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "tag-non-verbose",
			Command:     test.Command("manifest", "inspect", testutil.AlpineImage),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var manifest map[string]interface{}
				assert.NilError(t, json.Unmarshal([]byte(stdout), &manifest))

				validateManifestListFormat(t, manifest)

				if manifests, ok := manifest["manifests"]; ok {
					manifestsArray, ok := manifests.([]interface{})
					assert.Assert(t, ok)
					assert.Assert(t, len(manifestsArray) > 0)
					findAndValidateAmd64ManifestInList(t, manifestsArray)
				}
			}),
		},
		{
			Description: "tag-verbose",
			Command:     test.Command("manifest", "inspect", testutil.AlpineImage, "--verbose"),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var dockerEntries []interface{}
				assert.NilError(t, json.Unmarshal([]byte(stdout), &dockerEntries))
				assert.Assert(t, len(dockerEntries) > 0)

				findAndValidateAmd64DockerEntry(t, dockerEntries)
			}),
		},
		{
			Description: "digest-non-verbose",
			Command:     test.Command("manifest", "inspect", testutil.GetImageWithoutTag("alpine")+"@"+testutil.GetTestImageManifestDigest("alpine", "linux/amd64")),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var manifest map[string]interface{}
				assert.NilError(t, json.Unmarshal([]byte(stdout), &manifest))

				validateSingleManifest(t, manifest)
			}),
		},
		{
			Description: "digest-verbose",
			Command:     test.Command("manifest", "inspect", testutil.GetImageWithoutTag("alpine")+"@"+testutil.GetTestImageManifestDigest("alpine", "linux/amd64"), "--verbose"),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var entry map[string]interface{}
				assert.NilError(t, json.Unmarshal([]byte(stdout), &entry))

				validateDockerManifestEntry(t, entry, testutil.GetImageWithoutTag("alpine")+"@"+testutil.GetTestImageManifestDigest("alpine", "linux/amd64"))
			}),
		},
	}

	testCase.Run(t)
}

func validateManifestListFormat(t tig.T, manifest map[string]interface{}) {
	if schemaVersion, ok := manifest["schemaVersion"]; ok {
		assert.Equal(t, schemaVersion, 2.0) // JSON numbers are float64
	}
}

func findAndValidateAmd64ManifestInList(t tig.T, manifests []interface{}) {
	foundAmd64 := false
	for _, m := range manifests {
		manifestEntry, ok := m.(map[string]interface{})
		if !ok {
			continue
		}

		if platform, ok := manifestEntry["platform"].(map[string]interface{}); ok {
			if platform["architecture"] == "amd64" && platform["os"] == "linux" {
				if digest, ok := manifestEntry["digest"].(string); ok {
					assert.Equal(t, digest, testutil.GetTestImageManifestDigest("alpine", "linux/amd64"), "amd64 manifest digest should match expected value")
					foundAmd64 = true
					break
				}
			}
		}
	}
	assert.Assert(t, foundAmd64, "should find amd64 platform manifest")
}

func validateDockerManifestEntryFields(t tig.T, entry map[string]interface{}) {
	_, hasRef := entry["Ref"]
	_, hasDescriptor := entry["Descriptor"]
	_, hasRaw := entry["Raw"]
	_, hasSchemaV2Manifest := entry["SchemaV2Manifest"]

	assert.Assert(t, hasRef, "should have Ref field")
	assert.Assert(t, hasDescriptor, "should have Descriptor field")
	assert.Assert(t, hasRaw, "should have Raw field")
	assert.Assert(t, hasSchemaV2Manifest, "should have SchemaV2Manifest field")
}

func findAndValidateAmd64DockerEntry(t tig.T, dockerEntries []interface{}) {
	foundAmd64 := false
	for _, e := range dockerEntries {
		entry, ok := e.(map[string]interface{})
		assert.Assert(t, ok)

		validateDockerManifestEntryFields(t, entry)

		if descriptor, ok := entry["Descriptor"].(map[string]interface{}); ok {
			if platform, ok := descriptor["platform"].(map[string]interface{}); ok {
				if platform["architecture"] == "amd64" && platform["os"] == "linux" {
					// Verify manifest digest
					if digest, ok := descriptor["digest"].(string); ok {
						assert.Equal(t, digest, testutil.GetTestImageManifestDigest("alpine", "linux/amd64"), "amd64 manifest digest should match expected value")
					}

					// Verify config digest
					if schemaV2Manifest, ok := entry["SchemaV2Manifest"].(map[string]interface{}); ok {
						if config, ok := schemaV2Manifest["config"].(map[string]interface{}); ok {
							if configDigest, ok := config["digest"].(string); ok {
								assert.Equal(t, configDigest, testutil.GetTestImageConfigDigest("alpine", "linux/amd64"), "amd64 config digest should match expected value")
							}
						}
					}
					foundAmd64 = true
					break
				}
			}
		}
	}
	assert.Assert(t, foundAmd64, "should find amd64 platform entry")
}

func validateSingleManifest(t tig.T, manifest map[string]interface{}) {
	if schemaVersion, ok := manifest["schemaVersion"]; ok {
		assert.Equal(t, schemaVersion, 2.0)
	}

	assert.Equal(t, manifest["mediaType"], "application/vnd.docker.distribution.manifest.v2+json")

	if config, ok := manifest["config"]; ok {
		configMap, ok := config.(map[string]interface{})
		assert.Assert(t, ok)

		assert.Equal(t, configMap["digest"], testutil.GetTestImageConfigDigest("alpine", "linux/amd64"), "config digest should match expected value")
		assert.Equal(t, configMap["mediaType"], "application/vnd.docker.container.image.v1+json")
		assert.Equal(t, configMap["size"], 1472.0)
	}

	if layers, ok := manifest["layers"]; ok {
		layersArray, ok := layers.([]interface{})
		assert.Assert(t, ok)
		assert.Assert(t, len(layersArray) > 0, "should have at least one layer")
	}
}

func validateDockerManifestEntry(t tig.T, entry map[string]interface{}, expectedRef string) {
	validateDockerManifestEntryFields(t, entry)

	// Verify Ref contains the specific digest
	if ref, ok := entry["Ref"].(string); ok {
		assert.Equal(t, ref, expectedRef, "Ref should match expected value")
	}

	// Check descriptor
	if descriptor, ok := entry["Descriptor"].(map[string]interface{}); ok {
		assert.Equal(t, descriptor["digest"], testutil.GetTestImageManifestDigest("alpine", "linux/amd64"), "descriptor digest should match expected value")
		assert.Equal(t, descriptor["mediaType"], "application/vnd.docker.distribution.manifest.v2+json")
		assert.Equal(t, descriptor["size"], 528.0)

		if platform, ok := descriptor["platform"].(map[string]interface{}); ok {
			assert.Equal(t, platform["architecture"], "amd64")
			assert.Equal(t, platform["os"], "linux")
		}
	}

	// Verify SchemaV2Manifest config digest
	if schemaV2Manifest, ok := entry["SchemaV2Manifest"].(map[string]interface{}); ok {
		if config, ok := schemaV2Manifest["config"].(map[string]interface{}); ok {
			assert.Equal(t, config["digest"], testutil.GetTestImageConfigDigest("alpine", "linux/amd64"), "config digest should match expected value")
			assert.Equal(t, config["mediaType"], "application/vnd.docker.container.image.v1+json")
			assert.Equal(t, config["size"], 1472.0)
		}
	}

	// Verify Raw field decodes correctly
	if raw, ok := entry["Raw"].(string); ok {
		decodedRaw, err := base64.StdEncoding.DecodeString(raw)
		assert.NilError(t, err)

		var decodedManifest map[string]interface{}
		assert.NilError(t, json.Unmarshal(decodedRaw, &decodedManifest))

		if decodedConfig, ok := decodedManifest["config"].(map[string]interface{}); ok {
			assert.Equal(t, decodedConfig["digest"], testutil.GetTestImageConfigDigest("alpine", "linux/amd64"), "decoded config digest should match expected value")
		}
	}
}
