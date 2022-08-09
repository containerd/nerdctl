//go:build freebsd || linux
// +build freebsd linux

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
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/sirupsen/logrus"
)

// ProcessSplit splits volume mount information received through command line into respective source, destination
// and optsRaw fields according to the volume type chosen and returns them accordingly.
func ProcessSplit(s string, volStore volumestore.VolumeStore, res *Processed, src string, dst string, options []string) (string, string, []string, error) {
	split := strings.Split(s, ":")
	switch len(split) {
	case 1:
		dst = s
		res.AnonymousVolume = idgen.GenerateID()
		logrus.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
		anonVol, err := volStore.Create(res.AnonymousVolume, []string{})
		if err != nil {
			return "", "", nil, err
		}
		src = anonVol.Mountpoint
		res.Type = Volume
		return src, dst, nil, err
	case 2, 3:
		res.Type = Bind
		src, dst = split[0], split[1]
		if !strings.Contains(src, "/") {
			// assume src is a volume name
			res.Name = src
			vol, err := volStore.Get(src)
			if err != nil {
				if errors.Is(err, errdefs.ErrNotFound) {
					vol, err = volStore.Create(src, nil)
					if err != nil {
						return "", "", nil, err
					}
				} else {
					return "", "", nil, err
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
				return "", "", nil, err
			}
		}
		if !filepath.IsAbs(dst) {
			return "", "", nil, fmt.Errorf("expected an absolute path, got %q", dst)
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
			return "", "", nil, err
		}
		res.Opts = append(res.Opts, specOpts...)
		return src, dst, options, err
	default:
		return "", "", nil, fmt.Errorf("failed to parse %q", s)
	}
}
