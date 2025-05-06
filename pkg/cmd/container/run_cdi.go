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

package container

import (
	"context"

	"tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/containerd/containerd/v2/core/containers"
	cdispec "github.com/containerd/containerd/v2/pkg/cdi"
	"github.com/containerd/containerd/v2/pkg/oci"
)

// withCDIDevices creates the OCI runtime spec options for injecting CDI devices.
// Two options are returned: The first ensures that the CDI registry is initialized with
// refresh disabled, and the second injects the devices into the container.
func withCDIDevices(cdiSpecDirs []string, devices ...string) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
		if len(devices) == 0 {
			return nil
		}

		// We configure the CDI registry with the configured spec dirs and disable refresh.
		cdi.Configure(
			cdi.WithSpecDirs(cdiSpecDirs...),
			cdi.WithAutoRefresh(false),
		)

		return cdispec.WithCDIDevices(devices...)(ctx, client, c, s)
	}
}
