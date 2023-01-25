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

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/errutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/netwalker"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/sirupsen/logrus"
)

func Inspect(ctx context.Context, options types.NetworkInspectOptions) error {
	globalOptions := options.GOptions
	e, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath)

	if err != nil {
		return err
	}
	if options.Mode != "native" && options.Mode != "dockercompat" {
		return fmt.Errorf("unknown mode %q", options.Mode)
	}

	var result []interface{}
	var errs []error
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

	for _, name := range options.Networks {
		if name == "host" || name == "none" {
			errs = append(errs, fmt.Errorf("pseudo network %q cannot be inspected", name))
			continue
		}
		n, err := walker.Walk(ctx, name)
		if err != nil {
			errs = append(errs, err)
		} else if n == 0 {
			errs = append(errs, fmt.Errorf("no such network: %s", name))
		}
	}

	if len(result) > 0 {
		err = formatter.FormatSlice(options.Format, options.Stdout, result)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		for _, err := range errs {
			logrus.Error(err)
		}
		return errutil.NewExitCoderErr(1)
	}
	return nil
}
