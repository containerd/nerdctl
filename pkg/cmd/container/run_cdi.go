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
	"fmt"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/log"
	"tags.cncf.io/container-device-interface/pkg/cdi"
)

// withCDIDevices creates the OCI runtime spec options for injecting CDI devices.
// Two options are returned: The first ensures that the CDI registry is initialized with
// refresh disabled, and the second injects the devices into the container.
func withCDIDevices(cdiSpecDirs []string, devices ...string) oci.SpecOpts {
	return func(ctx context.Context, _ oci.Client, c *containers.Container, s *oci.Spec) error {
		if len(devices) == 0 {
			return nil
		}

		// We configure the CDI registry with the configured spec dirs and disable refresh.
		cdi.Configure(
			cdi.WithSpecDirs(cdiSpecDirs...),
			cdi.WithAutoRefresh(false),
		)

		// TODO: Call oci.WithCDIDevices(devices...) here once the dependencies have been updated.
		if err := cdi.Refresh(); err != nil {
			// We don't consider registry refresh failure a fatal error.
			// For instance, a dynamically generated invalid CDI Spec file for
			// any particular vendor shouldn't prevent injection of devices of
			// different vendors. CDI itself knows better and it will fail the
			// injection if necessary.
			log.G(ctx).Warnf("CDI registry refresh failed: %v", err)
		}

		if _, err := cdi.InjectDevices(s, devices...); err != nil {
			return fmt.Errorf("CDI device injection failed: %w", err)
		}

		// One crucial thing to keep in mind is that CDI device injection
		// might add OCI Spec environment variables, hooks, and mounts as
		// well. Therefore it is important that none of the corresponding
		// OCI Spec fields are reset up in the call stack once we return.
		return nil
	}
}
