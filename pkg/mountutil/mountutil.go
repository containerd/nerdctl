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
	Mount           specs.Mount
	AnonymousVolume string // name
	Type            string
	Opts            []oci.SpecOpts
}

func ProcessFlagV(s string, volStore volumestore.VolumeStore) (*Processed, error) {

	fstype := "nullfs"
	resp, err := ProcessSplit(s, volStore)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "freebsd" {
		fstype = "none"
		if runtime.GOOS == "windows" {
			fstype = ""
		}
		options = append(options, "rbind")
	}
	resp.Mount = specs.Mount{
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
		resp.Mount.Options = strutil.DedupeStrSlice(append(resp.Mount.Options, unpriv...))
	}
	return &resp, nil
}
