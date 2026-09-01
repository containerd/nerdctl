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

package attachmux

import (
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestSocketPathIsShortEnoughForSunPath(t *testing.T) {
	t.Parallel()

	// A unix socket path has to fit in sockaddr_un.sun_path, which is 108
	// bytes. A container ID is 64 hex characters, so the full ID cannot be part
	// of the path.
	id := strings.Repeat("a", 64)
	path := SocketPath("/var/lib/nerdctl/1935db59", "default", id)
	assert.Assert(t, len(path) < 100, "socket path %q is %d bytes", path, len(path))
	assert.Assert(t, strings.HasSuffix(path, ".sock"), "got %q", path)
}

func TestSocketPathIsStableAndDistinct(t *testing.T) {
	t.Parallel()

	first := SocketPath("/var/lib/nerdctl/1935db59", "default", strings.Repeat("a", 64))
	again := SocketPath("/var/lib/nerdctl/1935db59", "default", strings.Repeat("a", 64))
	assert.Equal(t, first, again)

	other := SocketPath("/var/lib/nerdctl/1935db59", "default", strings.Repeat("b", 64))
	assert.Assert(t, first != other)
}

func TestSocketPathSeparatesNamespaces(t *testing.T) {
	t.Parallel()

	// A data store's attach sockets share one flat directory and Listen removes
	// whatever is already at the path, so two containers with the same short ID
	// in different namespaces must not resolve to the same socket. nerdctl's own
	// IDs are random and 64 characters long, but nerdctl can attach to a
	// container created by another client, whose ID may be anything.
	first := SocketPath("/var/lib/nerdctl/1935db59", "default", "shell")
	other := SocketPath("/var/lib/nerdctl/1935db59", "staging", "shell")
	assert.Assert(t, first != other, "both namespaces resolved to %q", first)
}

func TestSocketPathSeparatesContainerdEndpoints(t *testing.T) {
	t.Parallel()

	// The data store path encodes the containerd address, and the socket lives
	// inside it, so two endpoints cannot collide.
	first := SocketPath("/var/lib/nerdctl/1935db59", "default", "shell")
	other := SocketPath("/var/lib/nerdctl/8fa1c02b", "default", "shell")
	assert.Assert(t, first != other, "both endpoints resolved to %q", first)
}

func TestSocketPathIsUnderTheDataStore(t *testing.T) {
	t.Parallel()

	// The broker derives this from the data store it is handed in its argv,
	// which is the only path it and its clients are guaranteed to agree on: the
	// shim spawns it with an environment of just CONTAINER_ID and
	// CONTAINER_NAMESPACE, so nothing XDG-based is available there.
	const dataStore = "/var/lib/nerdctl/1935db59"
	path := SocketPath(dataStore, "default", "abc")
	// Joined, not spelled out: the separator is the platform's.
	assert.Equal(t, filepath.Dir(path), filepath.Join(dataStore, "attach"))
	assert.Equal(t, len(filepath.Base(path)), socketNameLen+len(".sock"))
}
