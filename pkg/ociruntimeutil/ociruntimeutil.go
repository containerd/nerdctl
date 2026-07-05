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

// Package ociruntimeutil provides client-side utilities for inspecting OCI runtimes
// such as runc and crun.
package ociruntimeutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go/features"

	"github.com/containerd/log"
)

// BinaryFromRuntimeStr resolves the path of the OCI runtime binary from runtimeStr,
// which is the value of the `--runtime` flag of `nerdctl run`, e.g.,
// "" (default), "io.containerd.runc.v2", "crun", or "/usr/local/sbin/runc".
//
// An error is returned when the binary cannot be determined, e.g., for a shim
// like "io.containerd.kata.v2" that does not expose the runtime binary name.
//
// The resolution is a best-effort guess of the client and may not match the
// actual binary used by the containerd daemon, e.g., when the daemon overrides
// the BinaryName option of the "io.containerd.runc.v2" shim in its config.
func BinaryFromRuntimeStr(runtimeStr string) (string, error) {
	if runtimeStr == "" || strings.HasPrefix(runtimeStr, "io.containerd.runc.") {
		// The "io.containerd.runc.v2" shim executes "runc" from $PATH by default.
		return exec.LookPath("runc")
	}
	if strings.HasPrefix(runtimeStr, "io.containerd.") || runtimeStr == "wtf.sbk.runj.v1" {
		return "", fmt.Errorf("cannot determine the OCI runtime binary for runtime %q", runtimeStr)
	}
	// runtimeStr refers to a binary such as "crun" or "/usr/local/sbin/runc"
	// (consistent with generateRuntimeCOpts in pkg/cmd/container).
	binary, err := exec.LookPath(runtimeStr)
	if err != nil {
		return "", fmt.Errorf("cannot determine the OCI runtime binary for runtime %q: %w", runtimeStr, err)
	}
	return binary, nil
}

var (
	featuresCacheMu sync.Mutex
	featuresCache   = make(map[string]*features.Features) // key: the resolved binary path
)

// Features returns the parsed output of `<binary> features`.
// https://github.com/opencontainers/runtime-spec/blob/v1.2.1/features.md
//
// The `features` subcommand is supported by runc >= 1.1 and crun >= 1.8.6.
//
// The result is cached in the XDG cache directory (e.g., ~/.cache/nerdctl/oci-runtime-features),
// with the cache entry being invalidated when the binary is modified.
func Features(binary string) (*features.Features, error) {
	binPath, err := exec.LookPath(binary)
	if err != nil {
		return nil, err
	}
	realPath, err := filepath.EvalSymlinks(binPath)
	if err != nil {
		return nil, err
	}

	featuresCacheMu.Lock()
	defer featuresCacheMu.Unlock()
	if f, ok := featuresCache[realPath]; ok {
		return f, nil
	}

	cachePath, err := featuresCachePath(realPath)
	if err != nil {
		log.L.WithError(err).Debugf("failed to determine the cache path for the features of %q", realPath)
		cachePath = ""
	}
	if cachePath != "" {
		// The cache entry is valid only when it is newer than the binary.
		if stale, err := isCacheStale(cachePath, realPath); err == nil && !stale {
			if b, err := os.ReadFile(cachePath); err == nil {
				var f features.Features
				if err = json.Unmarshal(b, &f); err == nil {
					featuresCache[realPath] = &f
					return &f, nil
				}
				log.L.WithError(err).Warnf("failed to parse the cached OCI runtime features %q (ignored)", cachePath)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "features")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("failed to run `%s features`: %w", binPath, err)
	}
	var f features.Features
	if err = json.Unmarshal(out, &f); err != nil {
		return nil, fmt.Errorf("failed to parse the output of `%s features`: %w", binPath, err)
	}
	featuresCache[realPath] = &f
	if cachePath != "" {
		if err = writeFileAtomically(cachePath, out); err != nil {
			log.L.WithError(err).Debugf("failed to cache the OCI runtime features to %q (ignored)", cachePath)
		}
	}
	return &f, nil
}

// featuresCachePath returns the cache file path for the features of the binary.
func featuresCachePath(realPath string) (string, error) {
	// os.UserCacheDir returns $XDG_CACHE_HOME (or ~/.cache) on Linux.
	cacheHome, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(realPath))
	return filepath.Join(cacheHome, "nerdctl", "oci-runtime-features", hex.EncodeToString(h[:])+".json"), nil
}

// isCacheStale returns whether the cache file is older than the binary,
// i.e., the binary was modified after the cache entry was created.
func isCacheStale(cachePath, realPath string) (bool, error) {
	stCache, err := os.Stat(cachePath)
	if err != nil {
		return true, err
	}
	stBin, err := os.Stat(realPath)
	if err != nil {
		return true, err
	}
	return !stCache.ModTime().After(stBin.ModTime()), nil
}

func writeFileAtomically(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
