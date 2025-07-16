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
	"encoding/json"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/manifesttypes"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

const (
	testImageName = "alpine"
	testPlatform  = "linux/amd64"
)

type testData struct {
	imageName      string
	platform       string
	imageRef       string
	manifestDigest string
	configDigest   string
	rawData        string
}

func newTestData(imageName, platform string) *testData {
	return &testData{
		imageName:      imageName,
		platform:       platform,
		imageRef:       testutil.GetTestImage(imageName),
		manifestDigest: testutil.GetTestImageManifestDigest(imageName, platform),
		configDigest:   testutil.GetTestImageConfigDigest(imageName, platform),
		rawData:        testutil.GetTestImageRaw(imageName, platform),
	}
}

func (td *testData) imageWithDigest() string {
	return testutil.GetTestImageWithoutTag(td.imageName) + "@" + td.manifestDigest
}

func (td *testData) isAmd64Platform(platform *ocispec.Platform) bool {
	return platform != nil &&
		platform.Architecture == "amd64" &&
		platform.OS == "linux"
}

func TestManifestInspect(t *testing.T) {
	testCase := nerdtest.Setup()
	td := newTestData(testImageName, testPlatform)

	testCase.SubTests = []*test.Case{
		{
			Description: "tag-non-verbose",
			Command:     test.Command("manifest", "inspect", td.imageRef),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var manifest manifesttypes.DockerManifestListStruct
				assert.NilError(t, json.Unmarshal([]byte(stdout), &manifest))

				assert.Equal(t, manifest.SchemaVersion, testutil.GetTestImageSchemaVersion(td.imageName))
				assert.Equal(t, manifest.MediaType, testutil.GetTestImageMediaType(td.imageName))
				assert.Assert(t, len(manifest.Manifests) > 0)

				var foundManifest *ocispec.Descriptor
				for _, m := range manifest.Manifests {
					if td.isAmd64Platform(m.Platform) {
						foundManifest = &m
						break
					}
				}
				assert.Assert(t, foundManifest != nil, "should find amd64 platform manifest")
				assert.Equal(t, foundManifest.Digest.String(), td.manifestDigest)
				assert.Equal(t, foundManifest.MediaType, testutil.GetTestImagePlatformMediaType(td.imageName, td.platform))
			}),
		},
		{
			Description: "tag-verbose",
			Command:     test.Command("manifest", "inspect", td.imageRef, "--verbose"),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var entries []manifesttypes.DockerManifestEntry
				assert.NilError(t, json.Unmarshal([]byte(stdout), &entries))
				assert.Assert(t, len(entries) > 0)

				var foundEntry *manifesttypes.DockerManifestEntry
				for _, e := range entries {
					if td.isAmd64Platform(e.Descriptor.Platform) {
						foundEntry = &e
						break
					}
				}
				assert.Assert(t, foundEntry != nil, "should find amd64 platform entry")

				expectedRef := td.imageRef + "@" + td.manifestDigest
				assert.Equal(t, foundEntry.Ref, expectedRef)
				assert.Equal(t, foundEntry.Descriptor.Digest.String(), td.manifestDigest)
				assert.Equal(t, foundEntry.Descriptor.MediaType, testutil.GetTestImagePlatformMediaType(td.imageName, td.platform))
				assert.Equal(t, foundEntry.Raw, td.rawData)
			}),
		},
		{
			Description: "digest-non-verbose",
			Command:     test.Command("manifest", "inspect", td.imageWithDigest()),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var manifest manifesttypes.DockerManifestStruct
				assert.NilError(t, json.Unmarshal([]byte(stdout), &manifest))

				assert.Equal(t, manifest.SchemaVersion, testutil.GetTestImageSchemaVersion(td.imageName))
				assert.Equal(t, manifest.MediaType, testutil.GetTestImagePlatformMediaType(td.imageName, td.platform))
				assert.Equal(t, manifest.Config.Digest.String(), td.configDigest)
			}),
		},
		{
			Description: "digest-verbose",
			Command:     test.Command("manifest", "inspect", td.imageWithDigest(), "--verbose"),
			Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
				var entry manifesttypes.DockerManifestEntry
				assert.NilError(t, json.Unmarshal([]byte(stdout), &entry))

				assert.Equal(t, entry.Ref, td.imageWithDigest())
				assert.Equal(t, entry.Descriptor.Digest.String(), td.manifestDigest)
				assert.Equal(t, entry.Descriptor.MediaType, testutil.GetTestImagePlatformMediaType(td.imageName, td.platform))
				assert.Equal(t, entry.Raw, td.rawData)
			}),
		},
	}

	testCase.Run(t)
}
