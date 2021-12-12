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

package copyutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// copied from https://github.com/kubernetes/kubectl/blob/v0.23.4/pkg/cmd/cp/filespec.go#L126-L134 (v0.23.4)
func StripTrailingSlash(file string) string {
	if len(file) == 0 {
		return file
	}
	if file != "/" && strings.HasSuffix(string(file[len(file)-1]), "/") {
		return file[:len(file)-1]
	}
	return file
}

// SplitPathDirEntry splits the given path between its directory name and its
// basename. We assume that remote files are both linux and windows containers.
// So we first replace each slash ('/') character in path with a separator character
// then we clean the path.
// A trailing "." is preserved if the original path specified the current directory.
func SplitPathDirEntry(path string) (dir, base string) {
	cleanedPath := filepath.Clean(filepath.FromSlash(path))

	if filepath.Base(path) == "." {
		cleanedPath += string(os.PathSeparator) + "."
	}

	return filepath.Dir(cleanedPath), filepath.Base(cleanedPath)
}

// IsCurrentDir check whether the given path specifies
// a "current directory", i.e., the last path segment is `.`.
func IsCurrentDir(path string) bool {
	return filepath.Base(path) == "."
}

// HasTrailingPathSeparator returns whether the given
// path ends with the system's path separator character.
func HasTrailingPathSeparator(path string, sep string) bool {
	return len(path) > 0 && strings.HasSuffix(string(path[len(path)-1]), sep)
}

func IsRelative(base, target string) bool {
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return relative == "." || relative == StripPathShortcuts(relative)
}

// copied from https://github.com/kubernetes/kubectl/blob/v0.23.4/pkg/cmd/cp/filespec.go#L142-L161 (v0.23.4)
func StripPathShortcuts(p string) string {
	newPath := p
	trimmed := strings.TrimPrefix(newPath, "../")

	for trimmed != newPath {
		newPath = trimmed
		trimmed = strings.TrimPrefix(newPath, "../")
	}

	// trim leftover {".", ".."}
	if newPath == "." || newPath == ".." {
		newPath = ""
	}

	if len(newPath) > 0 && string(newPath[0]) == "/" {
		return newPath[1:]
	}

	return newPath
}

func TarBinary() (string, error) {
	return exec.LookPath("tar")
}
