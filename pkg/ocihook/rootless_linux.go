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

package ocihook

import (
	"context"

	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	rlkclient "github.com/rootless-containers/rootlesskit/pkg/api/client"
)

func exposePortsRootless(ctx context.Context, rlkClient rlkclient.Client, ports []gocni.PortMapping) error {
	pm, err := rootlessutil.NewRootlessCNIPortManager(rlkClient)
	if err != nil {
		return err
	}
	for _, p := range ports {
		if err := pm.ExposePort(ctx, p); err != nil {
			return err
		}
	}

	return nil
}

func unexposePortsRootless(ctx context.Context, rlkClient rlkclient.Client, ports []gocni.PortMapping) error {
	pm, err := rootlessutil.NewRootlessCNIPortManager(rlkClient)
	if err != nil {
		return err
	}
	for _, p := range ports {
		if err := pm.UnexposePort(ctx, p); err != nil {
			return err
		}
	}

	return nil
}
