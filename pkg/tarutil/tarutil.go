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

package tarutil

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/archive/tarheader"
	cfs "github.com/containerd/continuity/fs"
	"github.com/docker/docker/pkg/pools"
	"github.com/moby/sys/sequential"
	"github.com/sirupsen/logrus"
)

const (
	paxSchilyXattr          = "SCHILY.xattr."
	securityCapabilityXattr = "security.capability"
)

// FindTarBinary returns a path to the tar binary and whether it is GNU tar.
func FindTarBinary() (string, bool, error) {
	isGNU := func(exe string) bool {
		v, err := exec.Command(exe, "--version").Output()
		if err != nil {
			logrus.Warnf("Failed to detect whether %q is GNU tar or not", exe)
			return false
		}
		if !strings.Contains(string(v), "GNU tar") {
			logrus.Warnf("%q does not seem GNU tar", exe)
			return false
		}
		return true
	}
	if v := os.Getenv("TAR"); v != "" {
		if exe, err := exec.LookPath(v); err == nil {
			return exe, isGNU(exe), nil
		}
	}
	if exe, err := exec.LookPath("gnutar"); err == nil {
		return exe, true, nil
	}
	if exe, err := exec.LookPath("gtar"); err == nil {
		return exe, true, nil
	}
	if exe, err := exec.LookPath("tar"); err == nil {
		return exe, isGNU(exe), nil
	}
	return "", false, fmt.Errorf("failed to find `tar` binary")
}

type Tarballer struct {
	Buffer    *bufio.Writer
	TarWriter *tar.Writer
	seenFiles map[uint64]string
}

// TODO: Add tar options for compression, whiteout files, chown ..etc

func NewTarballer(writer io.Writer) *Tarballer {
	return &Tarballer{
		Buffer:    pools.BufioWriter32KPool.Get(nil),
		TarWriter: tar.NewWriter(writer),
		seenFiles: make(map[uint64]string),
	}
}

// TODO: Add unit test

// Tar creates an archive from the directory at `root`.
// Mostly copied over from https://github.com/containerd/containerd/blob/main/archive/tar.go#L552
func (tb *Tarballer) Tar(root string) error {
	defer func() error {
		pools.BufioWriter32KPool.Put(tb.Buffer)
		return tb.TarWriter.Close()
	}()
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to Lstat: %w", err)
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		header, err := FileInfoHeader(info, relPath, link)
		if err != nil {
			return err
		}
		inode, isHardlink := cfs.GetLinkInfo(info)

		if isHardlink {
			if oldpath, ok := tb.seenFiles[inode]; ok {
				header.Typeflag = tar.TypeLink
				header.Linkname = oldpath
				header.Size = 0
			} else {
				tb.seenFiles[inode] = relPath
			}
		}
		if capability, err := getxattr(path, securityCapabilityXattr); err != nil {
			return fmt.Errorf("failed to get capabilities xattr: %w", err)
		} else if len(capability) > 0 {
			if header.PAXRecords == nil {
				header.PAXRecords = map[string]string{}
			}
			header.PAXRecords[paxSchilyXattr+securityCapabilityXattr] = string(capability)
		}

		// TODO: Currently not setting UID/GID. Handle remapping UID/GID in container to that of host

		err = tb.TarWriter.WriteHeader(header)
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && header.Size > 0 {
			f, err := sequential.Open(path)
			if err != nil {
				return err
			}
			tb.Buffer.Reset(tb.TarWriter)
			defer tb.Buffer.Reset(tb.TarWriter)
			if _, err = io.Copy(tb.Buffer, f); err != nil {
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
			if err = tb.Buffer.Flush(); err != nil {
				return err
			}
		}
		return nil
	})
}

func FileInfoHeader(info os.FileInfo, path, link string) (*tar.Header, error) {
	header, err := tarheader.FileInfoHeaderNoLookups(info, link)
	if err != nil {
		return nil, err
	}
	header.Mode = int64(chmodTarEntry(os.FileMode(header.Mode)))
	header.Format = tar.FormatPAX
	header.ModTime = header.ModTime.Truncate(time.Second)
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}

	name := filepath.ToSlash(path)
	if info.IsDir() && !strings.HasSuffix(path, "/") {
		name += "/"
	}
	header.Name = name

	return header, nil
}
