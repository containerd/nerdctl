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
	"context"
	"encoding/json"
	"fmt"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/idutil/netwalker"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
)

func Inspect(ctx context.Context, options types.NetworkInspectOptions) error {
	globalOptions := options.GOptions
	e, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath, netutil.WithNamespace(options.GOptions.Namespace))

	if err != nil {
		return err
	}
	if options.Mode != "native" && options.Mode != "dockercompat" {
		return fmt.Errorf("unknown mode %q", options.Mode)
	}

	var result []interface{}
	walker := netwalker.NetworkWalker{
		Client: e,
		OnFound: func(ctx context.Context, found netwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			r := &native.Network{
				CNI:           json.RawMessage(found.Network.Bytes),
				NerdctlID:     found.Network.NerdctlID,
				NerdctlLabels: found.Network.NerdctlLabels,
				File:          found.Network.File,
			}
			switch options.Mode {
			case "native":
				result = append(result, r)
			case "dockercompat":
				compat, err := dockercompat.NetworkFromNative(r)
				if err != nil {
					return err
				}
				result = append(result, compat)
			}
			return nil
		},
	}

	// `network inspect` doesn't support pseudo network.
	err = walker.WalkAll(ctx, options.Networks, true, false)
	if len(result) > 0 {
		if formatErr := formatter.FormatSlice(options.Format, options.Stdout, result); formatErr != nil {
			log.G(ctx).Error(formatErr)
		}
	}
	return err
}
