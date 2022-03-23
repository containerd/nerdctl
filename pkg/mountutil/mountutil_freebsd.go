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
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/sirupsen/logrus"
	"path/filepath"
	"strings"
)

var (
	res      Processed
	src, dst string
	options  []string
)

func getUnprivilegedMountFlags(path string) ([]string, error) {
	m := []string{}
	return m, nil
}

// FreeBSD doesn't support bind mounts.
const DefaultPropagationMode = ""

// parseVolumeOptions parses specified optsRaw with using information of
// the volume type and the src directory when necessary.
func parseVolumeOptions(vType, src, optsRaw string) ([]string, []oci.SpecOpts, error) {
	var writeModeRawOpts []string
	for _, opt := range strings.Split(optsRaw, ",") {
		switch opt {
		case "rw":
			writeModeRawOpts = append(writeModeRawOpts, opt)
		case "ro":
			writeModeRawOpts = append(writeModeRawOpts, opt)
		case "":
			// NOP
		default:
			logrus.Warnf("unsupported volume option %q", opt)
		}
	}
	var opts []string
	if len(writeModeRawOpts) > 1 {
		return nil, nil, fmt.Errorf("duplicated read/write volume option: %+v", writeModeRawOpts)
	} else if len(writeModeRawOpts) > 0 && writeModeRawOpts[0] == "ro" {
		opts = append(opts, "ro")
	} // No need to return option when "rw"
	return opts, nil, nil
}

func ProcessFlagTmpfs(s string) (*Processed, error) {
	return nil, errdefs.ErrNotImplemented
}

func ProcessFlagMount(s string, volStore volumestore.VolumeStore) (*Processed, error) {
	return nil, errdefs.ErrNotImplemented
}
func ProcessSplit(s string, volStore volumestore.VolumeStore) (Processed, error) {
	split := strings.Split(s, ":")
	switch len(split) {
	case 1:
		dst = s
		res.AnonymousVolume = idgen.GenerateID()
		logrus.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
		anonVol, err := volStore.Create(res.AnonymousVolume, []string{})
		if err != nil {
			return res, err
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
						return res, err
					}
				} else {
					return res, err
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
				return res, err
			}
		}
		if !filepath.IsAbs(dst) {
			return res, fmt.Errorf("expected an absolute path, got %q", dst)
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
			return res, err
		}
		res.Opts = append(res.Opts, specOpts...)
	default:
		return res, fmt.Errorf("failed to parse %q", s)
	}
	return res, nil
}
