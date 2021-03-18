/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func ParseFlagV(s string, volumes map[string]native.Volume) (*specs.Mount, error) {
	split := strings.Split(s, ":")
	if len(split) < 2 || len(split) > 3 {
		return nil, errors.Errorf("failed to parse %q", s)
	}
	src, dst := split[0], split[1]
	if !strings.Contains(src, "/") {
		// assume src is a volume name
		vol, ok := volumes[src]
		if !ok {
			return nil, errors.Errorf("unknown volume name %q", src)
		}
		// src is now full path
		src = vol.Mountpoint
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
	ro := false
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
	m := &specs.Mount{
		Type:        "none",
		Source:      src,
		Destination: dst,
		Options:     []string{"rbind"},
	}
	if ro {
		m.Options = append(m.Options, "ro")
	}
	return m, nil
}
