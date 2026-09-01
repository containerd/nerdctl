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

package rootlessutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootlessCgroup2GroupPath(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "child_pid"), []byte("4321\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	want := "/wsl-user/distro-280/systemd/user.slice/user-1000.slice/user@1000.service/app.slice/containerd.service"
	got, err := rootlessCgroup2GroupPath(stateDir, func(pid int) (string, error) {
		if pid != 4321 {
			t.Fatalf("unexpected PID: %d", pid)
		}
		return want, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRootlessCgroup2GroupPathErrors(t *testing.T) {
	t.Run("missing child PID", func(t *testing.T) {
		_, err := rootlessCgroup2GroupPath(t.TempDir(), func(int) (string, error) {
			return "/user.slice", nil
		})
		if err == nil || !strings.Contains(err.Error(), "RootlessKit child PID") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("resolver failure", func(t *testing.T) {
		stateDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(stateDir, "child_pid"), []byte("4321"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := rootlessCgroup2GroupPath(stateDir, func(int) (string, error) {
			return "", errors.New("test failure")
		})
		if err == nil || !strings.Contains(err.Error(), "test failure") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		stateDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(stateDir, "child_pid"), []byte("4321"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := rootlessCgroup2GroupPath(stateDir, func(int) (string, error) {
			return "/outer/../user.slice", nil
		})
		if err == nil || !strings.Contains(err.Error(), "invalid cgroup v2 path") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
