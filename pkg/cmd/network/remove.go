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
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/errutil"
	"github.com/containerd/nerdctl/pkg/idutil/netwalker"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/sirupsen/logrus"
)

func Remove(ctx context.Context, client *containerd.Client, options types.NetworkRemoveOptions) error {
	e, err := netutil.NewCNIEnv(options.GOptions.CNIPath, options.GOptions.CNINetConfPath)
	if err != nil {
		return err
	}

	usedNetworkInfo, err := netutil.UsedNetworks(ctx, client)
	if err != nil {
		return err
	}

	walker := netwalker.NetworkWalker{
		Client: e,
		OnFound: func(ctx context.Context, found netwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if value, ok := usedNetworkInfo[found.Network.Name]; ok {
				return fmt.Errorf("network %q is in use by container %q", found.Req, value)
			}
			if found.Network.NerdctlID == nil {
				return fmt.Errorf("%s is managed outside nerdctl and cannot be removed", found.Req)
			}
			if found.Network.File == "" {
				return fmt.Errorf("%s is a pre-defined network and cannot be removed", found.Req)
			}
			if err := e.RemoveNetwork(found.Network); err != nil {
				return err
			}
			fmt.Fprintln(options.Stdout, found.Req)
			return nil
		},
	}

	code := 0
	for _, name := range options.Networks {
		if name == "host" || name == "none" {
			code = 1
			logrus.Errorf("pseudo network %q cannot be removed", name)
			continue
		}

		n, err := walker.Walk(ctx, name)
		if err != nil {
			code = 1
			logrus.Error(err)
			continue

		} else if n == 0 {
			code = 1
			logrus.Errorf("No such network: %s", name)
			continue
		}
	}

	// compatible with docker
	// ExitCodeError is to allow the program to exit with status code 1 without outputting an error message.
	if code != 0 {
		return errutil.NewExitCoderErr(code)
	}
	return nil
}
