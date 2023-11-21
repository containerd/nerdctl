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

package mountutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/userns"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	Bind   = "bind"
	Volume = "volume"
	Tmpfs  = "tmpfs"
)

type Processed struct {
	Type            string
	Mount           specs.Mount
	Name            string // name
	AnonymousVolume string // anonymous volume name
	Mode            string
	Opts            []oci.SpecOpts
}

func ProcessFlagV(s string, volStore volumestore.VolumeStore, createDir bool) (*Processed, error) {
	var (
		res      Processed
		src, dst string
		options  []string
	)

	s = strings.TrimLeft(s, ":")
	split := strings.Split(s, ":")
	switch len(split) {
	case 1:
		dst = s
		res.AnonymousVolume = idgen.GenerateID()
		log.L.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
		anonVol, err := volStore.Create(res.AnonymousVolume, []string{})
		if err != nil {
			return nil, fmt.Errorf("failed to create an anonymous volume %q: %w", res.AnonymousVolume, err)
		}
		src = anonVol.Mountpoint
		res.Type = Volume
	case 2, 3:
		res.Type = Bind
		src, dst = split[0], split[1]
		if !strings.Contains(src, "/") {
			// assume src is a volume name
			res.Name = src
			vol, err := volStore.Get(src, false)
			if err != nil {
				if errors.Is(err, errdefs.ErrNotFound) {
					vol, err = volStore.Create(src, nil)
					if err != nil {
						return nil, fmt.Errorf("failed to create volume %q: %w", src, err)
					}
				} else {
					return nil, fmt.Errorf("failed to get volume %q: %w", src, err)
				}
			}
			// src is now full path
			src = vol.Mountpoint
			res.Type = Volume
		}
		if !filepath.IsAbs(src) {
			log.L.Warnf("expected an absolute path, got a relative path %q (allowed for nerdctl, but disallowed for Docker, so unrecommended)", src)
			var err error
			src, err = filepath.Abs(src)
			if err != nil {
				return nil, fmt.Errorf("failed to get the absolute path of %q: %w", src, err)
			}
		}
		if createDir {
			if _, err := os.Stat(src); err != nil {
				if !os.IsNotExist(err) {
					return nil, fmt.Errorf("failed to stat %q: %w", src, err)
				}
				if err := os.MkdirAll(src, 0o755); err != nil {
					return nil, fmt.Errorf("failed to mkdir %q: %w", src, err)
				}
			}
		}

		if !filepath.IsAbs(dst) {
			return nil, fmt.Errorf("expected an absolute path, got %q", dst)
		}
		rawOpts := ""
		if len(split) == 3 {
			rawOpts = split[2]
		}
		res.Mode = rawOpts

		// always call parseVolumeOptions for bind mount to allow the parser to add some default options
		var err error
		var specOpts []oci.SpecOpts
		options, specOpts, err = parseVolumeOptions(res.Type, src, rawOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse volume options (%q, %q, %q): %w", res.Type, src, rawOpts, err)
		}
		res.Opts = append(res.Opts, specOpts...)
	default:
		return nil, fmt.Errorf("failed to parse %q", s)
	}

	fstype := "nullfs"
	if runtime.GOOS != "freebsd" {
		fstype = "none"
		found := false
		for _, opt := range options {
			switch opt {
			case "rbind", "bind":
				fstype = "bind"
				found = true
			}
			if found {
				break
			}
		}
		if !found {
			options = append(options, "rbind")
		}
	}
	res.Mount = specs.Mount{
		Type:        fstype,
		Source:      src,
		Destination: dst,
		Options:     options,
	}
	if userns.RunningInUserNS() {
		unpriv, err := UnprivilegedMountFlags(src)
		if err != nil {
			return nil, fmt.Errorf("failed to get unprivileged mount flags for %q: %w", src, err)
		}
		res.Mount.Options = strutil.DedupeStrSlice(append(res.Mount.Options, unpriv...))
	}
	return &res, nil
}
