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
	"fmt"
	"strings"

	"github.com/containerd/containerd/oci"
	"github.com/sirupsen/logrus"
)

func getUnprivilegedMountFlags(path string) ([]string, error) {
	m := []string{}
	return m, nil
}

// DefaultPropagationMode is the default propagation of mounts
// where user doesn't specify mount propagation explicitly.
// See also: https://github.com/moby/moby/blob/v20.10.7/volume/mounts/windows_parser.go#L440-L442
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
