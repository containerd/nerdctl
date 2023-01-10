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

package network

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/netutil"
)

func Inspect(options types.NetworkInspectCommandOptions, stdout io.Writer) error {
	e, err := netutil.NewCNIEnv(options.GOptions.CNIPath, options.GOptions.CNINetConfPath)
	if err != nil {
		return err
	}

	netMap, err := e.NetworkMap()
	if err != nil {
		return err
	}

	result := make([]interface{}, len(options.Networks))
	for i, name := range options.Networks {
		if name == "host" || name == "none" {
			return fmt.Errorf("pseudo network %q cannot be inspected", name)
		}
		l, ok := netMap[name]
		if !ok {
			return fmt.Errorf("no such network: %s", name)
		}

		r := &native.Network{
			CNI:           json.RawMessage(l.Bytes),
			NerdctlID:     l.NerdctlID,
			NerdctlLabels: l.NerdctlLabels,
			File:          l.File,
		}
		switch options.Mode {
		case "native":
			result[i] = r
		case "dockercompat":
			compat, err := dockercompat.NetworkFromNative(r)
			if err != nil {
				return err
			}
			result[i] = compat
		default:
			return fmt.Errorf("unknown mode %q", options.Mode)
		}
	}
	return formatter.FormatSlice(options.Format, stdout, result)
}
