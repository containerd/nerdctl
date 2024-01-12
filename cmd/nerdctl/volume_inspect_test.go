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

package main

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestVolumeInspectContainsLabels(t *testing.T) {
	t.Parallel()
	testVolume := testutil.Identifier(t)

	base := testutil.NewBase(t)
	base.Cmd("volume", "create", "--label", "tag=testVolume", testVolume).AssertOK()
	defer base.Cmd("volume", "rm", "-f", testVolume).Run()

	inspect := base.InspectVolume(testVolume)
	inspectNerdctlLabels := (*inspect.Labels)
	expected := make(map[string]string, 1)
	expected["tag"] = "testVolume"
	assert.DeepEqual(base.T, expected, inspectNerdctlLabels)
}

func TestVolumeInspectSize(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	testVolume := testutil.Identifier(t)
	base := testutil.NewBase(t)
	base.Cmd("volume", "create", testVolume).AssertOK()
	defer base.Cmd("volume", "rm", "-f", testVolume).Run()

	var size int64 = 1028
	createFileWithSize(t, testVolume, size)
	volumeWithSize := base.InspectVolume(testVolume, []string{"--size"}...)
	assert.Equal(t, volumeWithSize.Size, size)
}

func createFileWithSize(t *testing.T, volume string, bytes int64) {
	base := testutil.NewBase(t)
	v := base.InspectVolume(volume)
	token := make([]byte, bytes)
	rand.Read(token)
	err := os.WriteFile(filepath.Join(v.Mountpoint, "test-file"), token, 0644)
	assert.NilError(t, err)
}
