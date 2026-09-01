//go:build linux || freebsd

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
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
	path := SocketPath("/var/lib/nerdctl/1935db59", "default", "abc")
	assert.Equal(t, filepath.Dir(path), "/var/lib/nerdctl/1935db59/attach")
	assert.Equal(t, len(filepath.Base(path)), socketNameLen+len(".sock"))
}

func TestListenAndDial(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "attach.sock")

	b := NewBroker(true, nil)
	l, err := Listen(path)
	assert.NilError(t, err)
	t.Cleanup(func() { l.Close() })

	info, err := os.Stat(path)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0600))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = b.Serve(ctx, l) }()

	session, err := Dial(ctx, path)
	assert.NilError(t, err)
	t.Cleanup(func() { session.Close() })
	assert.Equal(t, session.TTY(), true)
}

func TestListenReplacesALeftoverSocket(t *testing.T) {
	t.Parallel()

	// A container that was killed leaves its socket behind, and bind would then
	// fail with EADDRINUSE. Go's unix listener unlinks the socket on Close, so
	// the leftover has to be staged by hand.
	path := filepath.Join(t.TempDir(), "attach.sock")
	assert.NilError(t, os.WriteFile(path, nil, 0600))

	l, err := Listen(path)
	assert.NilError(t, err)
	t.Cleanup(func() { l.Close() })
}

func TestProbe(t *testing.T) {
	t.Parallel()

	// Probe has to leave nothing behind: it runs on every `nerdctl run -it`.
	dataStore := t.TempDir()
	assert.NilError(t, Probe(dataStore))

	entries, err := os.ReadDir(socketDir(dataStore))
	assert.NilError(t, err)
	for _, e := range entries {
		assert.Assert(t, !strings.HasPrefix(e.Name(), "probe"), "Probe left %q behind", e.Name())
	}
}

func TestProbeIsRepeatable(t *testing.T) {
	t.Parallel()

	// Probe binds a real socket, so it has to clean up after itself well enough
	// to run again, which it does on every foreground `nerdctl run -it`.
	dataStore := t.TempDir()
	assert.NilError(t, Probe(dataStore))
	assert.NilError(t, Probe(dataStore))
}

func TestProbeIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	// Nothing stops two goroutines in one process from probing at once, and a
	// shared socket name would make one unlink the other's socket.
	dataStore := t.TempDir()
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() { errs[i] = Probe(dataStore) })
	}
	wg.Wait()
	for _, err := range errs {
		assert.NilError(t, err)
	}
}

func TestDialToNothing(t *testing.T) {
	t.Parallel()

	// Dial retries while waiting for the broker to bind, so it gives up only
	// when the context is done. Keep that short here.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	t.Cleanup(cancel)

	_, err := Dial(ctx, filepath.Join(t.TempDir(), "absent.sock"))
	assert.Assert(t, err != nil)
}
