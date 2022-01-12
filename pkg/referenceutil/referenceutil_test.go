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

package referenceutil

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestSuggestContainerName(t *testing.T) {
	const containerID = "16f6d167d4f4743e48affb86e7097222b7992b34a29dab5f8c10cd6a90cdd990"
	assert.Equal(t, "alpine-16f6d", SuggestContainerName("alpine", containerID))
	assert.Equal(t, "alpine-16f6d", SuggestContainerName("alpine:3.15", containerID))
	assert.Equal(t, "alpine-16f6d", SuggestContainerName("docker.io/library/alpine:3.15", containerID))
	assert.Equal(t, "alpine-16f6d", SuggestContainerName("docker.io/library/alpine:latest", containerID))
	assert.Equal(t, "ipfs-bafkr-16f6d", SuggestContainerName("bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze", containerID))
	assert.Equal(t, "ipfs-bafkr-16f6d", SuggestContainerName("ipfs://bafkreicq4dg6nkef5ju422ptedcwfz6kcvpvvhuqeykfrwq5krazf3muze", containerID))
	assert.Equal(t, "untitled-16f6d", SuggestContainerName("invalid://alpine", containerID))
	assert.Equal(t, "untitled-16f6d", SuggestContainerName("", containerID))
}
