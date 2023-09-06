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
	"fmt"
	"io"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/netutil"
)

func Create(options types.NetworkCreateOptions, stdout io.Writer) error {
	if options.CreateOptions.Subnet == "" {
		if options.CreateOptions.Gateway != "" || options.CreateOptions.IPRange != "" {
			return fmt.Errorf("cannot set gateway or ip-range without subnet, specify --subnet manually")
		}
	}

	e, err := netutil.NewCNIEnv(options.GOptions.CNIPath, options.GOptions.CNINetConfPath)
	if err != nil {
		return err
	}
	net, err := e.CreateNetwork(options.CreateOptions)
	if err != nil {
		if errdefs.IsAlreadyExists(err) {
			return fmt.Errorf("network with name %s already exists", options.CreateOptions.Name)
		}
		return err
	}
	_, err = fmt.Fprintln(stdout, *net.NerdctlID)
	return err
}
