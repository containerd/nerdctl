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
	"github.com/containerd/log"
)

// detectGPUVendorFromCDI detects the first available GPU vendor from CDI cache.
// Returns empty string if no known vendor is found.
func detectGPUVendorFromCDI() string {
	cache := cdi.GetDefaultCache()
	availableVendors := cache.ListVendors()
	knownGPUVendors := []string{"nvidia.com", "amd.com"}
	for _, known := range knownGPUVendors {
		for _, available := range availableVendors {
			if known == available {
				return known
			}
		}
	}

	return ""
}

// withStaticCDIRegistry inits the CDI registry with given spec dirs
// and disables auto-refresh.
func withStaticCDIRegistry(cdiSpecDirs []string) oci.SpecOpts {
	return func(ctx context.Context, _ oci.Client, _ *containers.Container, _ *oci.Spec) error {
		_ = cdi.Configure(
			cdi.WithSpecDirs(cdiSpecDirs...),
			cdi.WithAutoRefresh(false),
		)
		if err := cdi.Refresh(); err != nil {
			// We don't consider registry refresh failure a fatal error.
			// For instance, a dynamically generated invalid CDI Spec file for
			// any particular vendor shouldn't prevent injection of devices of
			// different vendors. CDI itself knows better and it will fail the
			// injection if necessary.
			log.L.Warnf("CDI cache refresh failed: %v", err)
		}
		return nil
	}
}

// withCDIDevices creates the OCI runtime spec options for injecting CDI devices.
func withCDIDevices(devices ...string) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
		if len(devices) == 0 {
			return nil
		}
		return cdispec.WithCDIDevices(devices...)(ctx, client, c, s)
	}
}

// withGPUs creates the OCI runtime spec options for injecting GPUs via CDI.
// It parses the given GPU options and converts them to CDI device IDs.
// withCDIDevices is then used to perform the actual injection.
func withGPUs(gpuOpts ...string) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
		if len(gpuOpts) == 0 {
			return nil
		}
		cdiDevices, err := parseGPUOpts(gpuOpts)
		if err != nil {
			return err
		}
		return withCDIDevices(cdiDevices...)(ctx, client, c, s)
	}
}
