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
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/sys"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/sirupsen/logrus"
)

const (
	Bind   = "bind"
	Volume = "volume"
	Tmpfs  = "tmpfs"
)

type Processed struct {
	Mount           specs.Mount
	AnonymousVolume string // name
	Type            string
	Opts            []oci.SpecOpts
}

func ProcessFlagV(s string, volStore volumestore.VolumeStore) (*Processed, error) {
	var (
		res      Processed
		src, dst string
		options  []string
	)

	split := strings.Split(s, ":")

	if runtime.GOOS == "windows" {
		switch len(split) {
		case 2: //For anonymous volumes in Windows
			dst = s
			res.AnonymousVolume = idgen.GenerateID()
			logrus.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
			anonVol, err := volStore.Create(res.AnonymousVolume, []string{})
			if err != nil {
				return nil, err
			}
			src = anonVol.Mountpoint
			res.Type = Volume
		case 3, 4, 5:
			rawOpts := ""
			res.Type = Bind
			if len(split) == 3 {
				src, dst = split[0], split[1]+":"+split[2]
			} else if len(split) == 4 {
				if strings.Contains(split[3], "\\") || strings.Contains(split[2], "/") {
					src = split[0] + ":" + split[1]
					dst = split[2] + ":" + split[3]
				} else {
					src = split[0]
					dst = split[1] + ":" + split[2]
					rawOpts = split[3]
				}

			} else if len(split) == 5 {
				src = split[0] + ":" + split[1]
				dst = split[2] + ":" + split[3]
				rawOpts = split[4]
			}
			if !strings.Contains(src, "/") && !strings.Contains(src, "\\") {
				// assume src is a volume name
				vol, err := volStore.Get(src)
				if err != nil {
					if errors.Is(err, errdefs.ErrNotFound) {
						vol, err = volStore.Create(src, nil)
						if err != nil {
							return nil, err
						}
						src = vol.Mountpoint
						res.Type = Volume
					} else {
						return nil, err
					}
				} else {
					// src is now full path
					src = vol.Mountpoint
				}
			}
			if !filepath.IsAbs(src) {
				logrus.Warnf("expected an absolute path, got a relative path %q (allowed for nerdctl, but disallowed for Docker, so unrecommended)", src)
				var err error
				src, err = filepath.Abs(src)
				if err != nil {
					return nil, err
				}
			}
			if !filepath.IsAbs(dst) {
				return nil, fmt.Errorf("expected an absolute path, got %q", dst)
			}

			// always call parseVolumeOptions for bind mount to allow the parser to add some default options
			var err error
			var specOpts []oci.SpecOpts
			options, specOpts, err = parseVolumeOptions(res.Type, src, rawOpts)
			if err != nil {
				return nil, err
			}
			res.Opts = append(res.Opts, specOpts...)
		default:
			return nil, fmt.Errorf("failed to parse %q", s)
		}
	} else {
		switch len(split) {
		case 1:
			dst = s
			res.AnonymousVolume = idgen.GenerateID()
			logrus.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
			anonVol, err := volStore.Create(res.AnonymousVolume, []string{})
			if err != nil {
				return nil, err
			}
			src = anonVol.Mountpoint
			res.Type = Volume
		case 2, 3:
			res.Type = Bind
			src, dst = split[0], split[1]
			if !strings.Contains(src, "/") {
				// assume src is a volume name
				vol, err := volStore.Get(src)
				if err != nil {
					if errors.Is(err, errdefs.ErrNotFound) {
						vol, err = volStore.Create(src, nil)
						if err != nil {
							return nil, err
						}
					} else {
						return nil, err
					}
				}
				// src is now full path
				src = vol.Mountpoint
				res.Type = Volume
			}
			if !filepath.IsAbs(src) {
				logrus.Warnf("expected an absolute path, got a relative path %q (allowed for nerdctl, but disallowed for Docker, so unrecommended)", src)
				var err error
				src, err = filepath.Abs(src)
				if err != nil {
					return nil, err
				}
			}
			if !filepath.IsAbs(dst) {
				return nil, fmt.Errorf("expected an absolute path, got %q", dst)
			}
			rawOpts := ""
			if len(split) == 3 {
				rawOpts = split[2]
			}

			// always call parseVolumeOptions for bind mount to allow the parser to add some default options
			var err error
			var specOpts []oci.SpecOpts
			options, specOpts, err = parseVolumeOptions(res.Type, src, rawOpts)
			if err != nil {
				return nil, err
			}
			res.Opts = append(res.Opts, specOpts...)
		default:
			return nil, fmt.Errorf("failed to parse %q", s)
		}
	}
	fstype := "nullfs"
	if runtime.GOOS != "freebsd" {
		fstype = ""
		options = append(options, "rbind")
	}
	res.Mount = specs.Mount{
		Type:        fstype,
		Source:      src,
		Destination: dst,
		Options:     options,
	}
	if sys.RunningInUserNS() {
		unpriv, err := getUnprivilegedMountFlags(src)
		if err != nil {
			return nil, err
		}
		res.Mount.Options = strutil.DedupeStrSlice(append(res.Mount.Options, unpriv...))
	}
	return &res, nil
}
