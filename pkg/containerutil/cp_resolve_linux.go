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

package containerutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd/v2/pkg/oci"
)

// volumeNameLen returns length of the leading volume name on Windows.
// It returns 0 elsewhere.
// FIXME: whenever we will want to port cp to windows, we will need the windows implementation of volumeNameLen
func volumeNameLen(_ string) int {
	return 0
}

var (
	errDoesNotExist           = errors.New("resource does not exist")                                            // when a path parent dir does not exist
	errIsNotADir              = errors.New("is not a dir")                                                       // when a path is a file, ending with path separator
	errCannotResolvePathNoCwd = errors.New("unable to resolve path against undefined current working directory") // relative host path, no cwd
)

// pathSpecifier represents a path to be used by cp
// besides exposing relevant properties (endsWithSeparator, etc), it also provides a fully resolved *host* path to
// access the resource
type pathSpecifier struct {
	originalPath         string
	endsWithSeparator    bool
	endsWithSeparatorDot bool
	exists               bool
	isADir               bool
	readOnly             bool
	resolvedPath         string
}

// getPathSpecFromHost builds a pathSpecifier from a host location
// errors with errDoesNotExist, errIsNotADir, "EvalSymlinks: too many links", or other hard filesystem errors from lstat/stat
func getPathSpecFromHost(originalPath string) (*pathSpecifier, error) {
	pathSpec := &pathSpecifier{
		originalPath:         originalPath,
		endsWithSeparator:    strings.HasSuffix(originalPath, string(os.PathSeparator)),
		endsWithSeparatorDot: filepath.Base(originalPath) == ".",
	}

	path := originalPath

	// Path may still be relative at this point. If it is, figure out getwd.
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, errors.Join(errCannotResolvePathNoCwd, err)
		}
		path = cwd + string(os.PathSeparator) + path
	}

	// Try to fully resolve the path
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if errors.Is(err, syscall.ENOTDIR) {
			return nil, errors.Join(errIsNotADir, err)
		}

		// Other errors:
		// - "EvalSymlinks: too many links"
		// - any other error coming from lstat
		return nil, err
	}

	pathSpec.exists = err == nil

	// Ensure the parent exists if the path itself does not
	if !pathSpec.exists {
		// Try the parent - obtain it by removing any trailing / or /., then the base
		cleaned := strings.TrimRight(strings.TrimSuffix(path, string(os.PathSeparator)+"."), string(os.PathSeparator))
		for len(cleaned) < len(path) {
			path = cleaned
			cleaned = strings.TrimRight(strings.TrimSuffix(path, string(os.PathSeparator)+"."), string(os.PathSeparator))
		}

		base := filepath.Base(path)
		path = strings.TrimSuffix(path, string(os.PathSeparator)+base)

		// Resolve it
		resolvedPath, err = filepath.EvalSymlinks(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, errors.Join(errDoesNotExist, err)
			} else if errors.Is(err, syscall.ENOTDIR) {
				return nil, errors.Join(errIsNotADir, err)
			}

			return nil, err
		}

		resolvedPath = filepath.Join(resolvedPath, base)
	} else {
		// If it exists, we can check if it is a dir
		var st os.FileInfo
		st, err = os.Stat(path)
		if err != nil {
			return nil, err
		}
		pathSpec.isADir = st.IsDir()
	}

	pathSpec.resolvedPath = resolvedPath

	return pathSpec, nil
}

// getPathSpecFromHost builds a pathSpecifier from a container location
func getPathSpecFromContainer(originalPath string, conSpec *oci.Spec, containerHostRoot string) (*pathSpecifier, error) {
	pathSpec := &pathSpecifier{
		originalPath:         originalPath,
		endsWithSeparator:    strings.HasSuffix(originalPath, string(os.PathSeparator)),
		endsWithSeparatorDot: filepath.Base(originalPath) == ".",
	}

	path := originalPath

	// Path may still be relative at this point. If it is, join it to the root
	// NOTE: this is specifically called out in the docker reference. Paths in the container are assumed
	// relative to the root, and not to the current (container) working directory.
	// Though this seems like a questionable decision, it is set.
	if !filepath.IsAbs(path) {
		path = string(os.PathSeparator) + path
	}

	// Now, fully resolve the path - resolving all symlinks and cleaning-up the end result, following across mounts
	pathResolver := newResolver(conSpec, containerHostRoot)
	resolvedContainerPath, err := pathResolver.resolvePath(path)

	// Errors we get from that are from Lstat or Readlink
	// Either the object does not exist, or we have a dangling symlink, or otherwise hosed filesystem entries
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if errors.Is(err, syscall.ENOTDIR) {
			return nil, errors.Join(errIsNotADir, err)
		}

		// errors.New("EvalSymlinks: too many links")
		// other errors would come from lstat
		return nil, err
	}

	pathSpec.exists = err == nil

	// If the resource does not exist
	if !pathSpec.exists {
		// Try the parent
		cleaned := strings.TrimRight(strings.TrimSuffix(path, string(os.PathSeparator)+"."), string(os.PathSeparator))
		for len(cleaned) < len(path) {
			path = cleaned
			cleaned = strings.TrimRight(strings.TrimSuffix(path, string(os.PathSeparator)+"."), string(os.PathSeparator))
		}

		base := filepath.Base(path)
		path = strings.TrimSuffix(path, string(os.PathSeparator)+base)

		resolvedContainerPath, err = pathResolver.resolvePath(path)

		// Error? That is the end
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, errors.Join(errDoesNotExist, err)
			} else if errors.Is(err, syscall.ENOTDIR) {
				return nil, errors.Join(errIsNotADir, err)
			}

			return nil, err
		}

		resolvedContainerPath = filepath.Join(resolvedContainerPath, base)
	}

	// Now, finally get the location of the fully resolved containerPath (in the root? in a volume?)
	containerMount, relativePath := pathResolver.getMount(resolvedContainerPath)
	pathSpec.resolvedPath = filepath.Join(containerMount.hostPath, relativePath)
	// If the endpoint is readonly, flag it as such
	if containerMount.readonly {
		pathSpec.readOnly = true
	}

	// If it exists, we can check if it is a dir
	if pathSpec.exists {
		var st os.FileInfo
		st, err = os.Stat(pathSpec.resolvedPath)
		if err != nil {
			return nil, err
		}

		pathSpec.isADir = st.IsDir()
	}

	return pathSpec, nil
}

// resolver provides methods to fully resolve any given container given path to a host location
// accounting for rootfs and mounts location
type resolver struct {
	root     *specs.Root
	mounts   []specs.Mount
	hostRoot string
}

// locator represents a container mount
type locator struct {
	containerPath string
	hostPath      string
	readonly      bool
}

func isParent(child []string, candidate []string) (bool, []string) {
	if len(child) < len(candidate) {
		return false, child
	}
	return slices.Equal(child[0:len(candidate)], candidate), child[len(candidate):]
}

// newResolver returns a resolver struct
func newResolver(conSpec *oci.Spec, hostRoot string) *resolver {
	return &resolver{
		root:     conSpec.Root,
		mounts:   conSpec.Mounts,
		hostRoot: hostRoot,
	}
}

// pathOnHost will return the *host* location of a container path, accounting for volumes.
// The provided path must be fully resolved, as returned by `resolvePath`.
func (res *resolver) pathOnHost(path string) string {
	hostRoot := res.hostRoot
	path = filepath.Clean(path)
	itemized := strings.Split(path, string(os.PathSeparator))

	containerRoot := "/"
	sub := itemized

	for _, mnt := range res.mounts {
		if candidateIsParent, subPath := isParent(itemized, strings.Split(mnt.Destination, string(os.PathSeparator))); candidateIsParent {
			if len(mnt.Destination) > len(containerRoot) {
				containerRoot = mnt.Destination
				hostRoot = mnt.Source
				sub = subPath
			}
		}
	}

	return filepath.Join(append([]string{hostRoot}, sub...)...)
}

// getMount returns the mount locator for a given fully-resolved path, along with the corresponding subpath of the path
// relative to the locator
func (res *resolver) getMount(path string) (*locator, string) {
	itemized := strings.Split(path, string(os.PathSeparator))

	loc := &locator{
		containerPath: "/",
		hostPath:      res.hostRoot,
		readonly:      res.root.Readonly,
	}

	sub := itemized

	for _, mnt := range res.mounts {
		if candidateIsParent, subPath := isParent(itemized, strings.Split(mnt.Destination, string(os.PathSeparator))); candidateIsParent {
			if len(mnt.Destination) > len(loc.containerPath) {
				loc.readonly = false
				for _, option := range mnt.Options {
					if option == "ro" {
						loc.readonly = true
					}
				}
				loc.containerPath = mnt.Destination
				loc.hostPath = mnt.Source
				sub = subPath
			}
		}
	}

	return loc, filepath.Join(sub...)
}

// resolvePath is adapted from https://cs.opensource.google/go/go/+/go1.23.0:src/path/filepath/path.go;l=147
// The (only) changes are on Lstat and ReadLink, which are fed the actual host path, that is computed by `res.pathOnHost`
func (res *resolver) resolvePath(path string) (string, error) {
	volLen := volumeNameLen(path)
	pathSeparator := string(os.PathSeparator)

	if volLen < len(path) && os.IsPathSeparator(path[volLen]) {
		volLen++
	}
	vol := path[:volLen]
	dest := vol
	linksWalked := 0
	//nolint:ineffassign
	for start, end := volLen, volLen; start < len(path); start = end {
		for start < len(path) && os.IsPathSeparator(path[start]) {
			start++
		}
		end = start
		for end < len(path) && !os.IsPathSeparator(path[end]) {
			end++
		}

		// On Windows, "." can be a symlink.
		// We look it up, and use the value if it is absolute.
		// If not, we just return ".".
		//nolint:staticcheck
		isWindowsDot := runtime.GOOS == "windows" && path[volumeNameLen(path):] == "."

		// The next path component is in path[start:end].
		if end == start {
			// No more path components.
			break
		} else if path[start:end] == "." && !isWindowsDot {
			// Ignore path component ".".
			continue
		} else if path[start:end] == ".." {
			// Back up to previous component if possible.
			// Note that volLen includes any leading slash.

			// Set r to the index of the last slash in dest,
			// after the volume.
			var r int
			for r = len(dest) - 1; r >= volLen; r-- {
				if os.IsPathSeparator(dest[r]) {
					break
				}
			}
			if r < volLen || dest[r+1:] == ".." {
				// Either path has no slashes
				// (it's empty or just "C:")
				// or it ends in a ".." we had to keep.
				// Either way, keep this "..".
				if len(dest) > volLen {
					dest += pathSeparator
				}
				dest += ".."
			} else {
				// Discard everything since the last slash.
				dest = dest[:r]
			}
			continue
		}

		// Ordinary path component. Add it to result.

		if len(dest) > volumeNameLen(dest) && !os.IsPathSeparator(dest[len(dest)-1]) {
			dest += pathSeparator
		}

		dest += path[start:end]

		// Resolve symlink.
		hostPath := res.pathOnHost(dest)
		fi, err := os.Lstat(hostPath)
		if err != nil {
			return "", err
		}

		if fi.Mode()&fs.ModeSymlink == 0 {
			if !fi.Mode().IsDir() && end < len(path) {
				return "", syscall.ENOTDIR
			}
			continue
		}

		// Found symlink.
		linksWalked++
		if linksWalked > 255 {
			return "", errors.New("EvalSymlinks: too many links")
		}

		link, err := os.Readlink(hostPath)
		if err != nil {
			return "", err
		}

		if isWindowsDot && !filepath.IsAbs(link) {
			// On Windows, if "." is a relative symlink,
			// just return ".".
			break
		}

		path = link + path[end:]

		v := volumeNameLen(link)
		if v > 0 {
			// Symlink to drive name is an absolute path.
			if v < len(link) && os.IsPathSeparator(link[v]) {
				v++
			}
			vol = link[:v]
			dest = vol
			end = len(vol)
		} else if len(link) > 0 && os.IsPathSeparator(link[0]) {
			// Symlink to absolute path.
			dest = link[:1]
			end = 1
			vol = link[:1]
			volLen = 1
		} else {
			// Symlink to relative path; replace last
			// path component in dest.
			var r int
			for r = len(dest) - 1; r >= volLen; r-- {
				if os.IsPathSeparator(dest[r]) {
					break
				}
			}
			if r < volLen {
				dest = vol
			} else {
				dest = dest[:r]
			}
			end = 0
		}
	}
	return filepath.Clean(dest), nil
}
