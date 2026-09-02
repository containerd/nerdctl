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

func TestDialToNothingFailsAtOnce(t *testing.T) {
	t.Parallel()

	// Attaching to a container that has been running for a while must not stall
	// the CLI: a missing socket is proof there is no broker, not a race.
	start := time.Now()
	_, err := Dial(context.Background(), filepath.Join(t.TempDir(), "absent.sock"))
	assert.Assert(t, err != nil)
	assert.Assert(t, time.Since(start) < time.Second, "Dial waited %s", time.Since(start))
}

func TestDialStartingWaitsForTheBroker(t *testing.T) {
	t.Parallel()

	// The caller has just created the task, and containerd does not wait for
	// the logging process it spawns, so the socket may not be there yet.
	path := filepath.Join(t.TempDir(), "attach.sock")
	b := NewBroker(true, nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		l, err := Listen(path)
		if err != nil {
			return
		}
		go b.Serve(context.Background(), l)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	session, err := DialStarting(ctx, path)
	assert.NilError(t, err)
	t.Cleanup(func() { session.Close() })
}

func TestDialStartingGivesUpEventually(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	t.Cleanup(cancel)

	_, err := DialStarting(ctx, filepath.Join(t.TempDir(), "absent.sock"))
	assert.Assert(t, err != nil)
}

func TestListenRemovesOnlyItsOwnSocket(t *testing.T) {
	t.Parallel()

	// Two brokers can briefly overlap for one container while the restart
	// monitor swaps tasks. The one that exits second must not unlink the live
	// socket of the one that took the path over.
	path := filepath.Join(t.TempDir(), "attach.sock")

	first, err := Listen(path)
	assert.NilError(t, err)

	second, err := Listen(path)
	assert.NilError(t, err)
	t.Cleanup(func() { second.Close() })

	// The old broker exits last.
	assert.NilError(t, first.Close())

	_, err = os.Stat(path)
	assert.NilError(t, err, "the surviving broker lost its socket")

	// And a broker that is alone does clean up after itself.
	assert.NilError(t, second.Close())
	_, err = os.Stat(path)
	assert.Assert(t, os.IsNotExist(err), "the socket outlived its broker")
}
