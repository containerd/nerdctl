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
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/sys"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"

	"runtime"
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

func ProcessFlagV(s string, volStore volumestore.VolumeStore) (*Processed, error) {
	var (
		res      Processed
		src, dst string
		options  []string
		err      error
	)

	src, dst, options, err = ProcessSplit(s, volStore, &res, src, dst, options)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "freebsd" {
		found := false
		for _, opt := range options {
			switch opt {
			case "rbind", "bind":
				found = true
				break
			}
		}
		if !found {
			options = append(options, "rbind")
		}
	}
	res.Mount = specs.Mount{
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
