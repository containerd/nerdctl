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

// Package attachmux lets any number of sessions share a container's stdio.
//
// The server side runs inside the process that owns the container's stdio, its
// internal logging process, and the client side runs in every `nerdctl attach`,
// `nerdctl run -it` and `nerdctl start -a`.
package attachmux

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
)

// ErrUnsupported is returned on platforms where the attach transport is not
// implemented yet. Callers fall back to attaching directly to the container's
// stdio, which allows a single session at a time. It lives here rather than in
// socket_unsupported.go because callers test for it on every platform.
var ErrUnsupported = errors.New("attachmux: multi-session attach is not supported on this platform")

// socketNameLen is how many hex characters of the digest go into a socket file
// name. A unix socket path has to fit in sockaddr_un.sun_path (108 bytes), and
// the data store path plus a 64 character container ID does not, hence the
// digest.
const socketNameLen = 16

// socketDir returns the directory holding a data store's attach sockets.
func socketDir(dataStore string) string {
	return filepath.Join(dataStore, "attach")
}

// SocketPath returns the attach socket path for a container.
//
// It lives under the container's data store rather than in a runtime directory,
// because the data store is the one location the broker and its clients can
// agree on. The broker runs inside the logging process, which containerd's shim
// spawns with an environment of exactly CONTAINER_ID and CONTAINER_NAMESPACE
// (cmd/containerd-shim-runc-v2/process/io_util.go, NewBinaryCmd): there is no
// XDG_RUNTIME_DIR there, and rootlessutil.XDGRuntimeDir would fail outright.
// The data store, by contrast, is handed to that process in its argv, and
// nerdctl already relies on it being the same path on both sides for the log
// files themselves.
//
// The name is a digest rather than a prefix of the ID, because all of a data
// store's attach sockets share one flat directory and Listen removes whatever
// is already at the path, so a collision would silently hand one container's
// terminal to another. The namespace is in the digest because nerdctl can
// attach to a container created by another client, whose ID may be short and
// identical to one elsewhere; nerdctl's own IDs are 64 random hex characters
// (see pkg/idgen). The data store no longer needs to be, since it is the
// directory.
//
// This is a pure function of its arguments, and it is how both the broker and
// every client find the socket: nothing about it is recorded on disk. See the
// head of Task 4 for why there is no pointer file.
func SocketPath(dataStore, ns, id string) string {
	sum := sha256.Sum256([]byte(ns + "\x00" + id))
	return filepath.Join(socketDir(dataStore), hex.EncodeToString(sum[:])[:socketNameLen]+".sock")
}
