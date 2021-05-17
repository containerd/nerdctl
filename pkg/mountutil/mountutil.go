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
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/sys"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	Bind   = "bind"
	Volume = "volume"
)

type Processed struct {
	Mount           specs.Mount
	AnonymousVolume string // name
	Type            string
}

func ProcessFlagV(s string, volStore volumestore.VolumeStore) (*Processed, error) {
	var (
		res      Processed
		src, dst string
		ro       bool
	)
	split := strings.Split(s, ":")
	switch len(split) {
	case 1:
		dst = s
		res.AnonymousVolume = idgen.GenerateID()
		logrus.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
		anonVol, err := volStore.Create(res.AnonymousVolume)
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
				return nil, err
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
			return nil, errors.Errorf("expected an absolute path, got %q", dst)
		}
		if len(split) == 3 {
			opts := strings.Split(split[2], ",")
			for _, opt := range opts {
				switch opt {
				case "rw":
					// NOP
				case "ro":
					ro = true
				default:
					logrus.Warnf("unsupported volume option %q", opt)
				}
			}
		}
	default:
		return nil, errors.Errorf("failed to parse %q", s)
	}
	res.Mount = specs.Mount{
		Type:        "none",
		Source:      src,
		Destination: dst,
		Options:     []string{"rbind"},
	}
	if ro {
		res.Mount.Options = append(res.Mount.Options, "ro")
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
