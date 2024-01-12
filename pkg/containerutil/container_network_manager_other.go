//go:build darwin || freebsd || netbsd || openbsd

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

package containerutil

import (
	"context"
	"fmt"
	"runtime"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

type cniNetworkManagerPlatform struct {
}

// Verifies that the internal network settings are correct.
func (m *cniNetworkManager) VerifyNetworkOptions(_ context.Context) error {
	return fmt.Errorf("CNI networking currently unsupported on %s", runtime.GOOS)
}

// Performs setup actions required for the container with the given ID.
func (m *cniNetworkManager) SetupNetworking(_ context.Context, _ string) error {
	return fmt.Errorf("CNI networking currently unsupported on %s", runtime.GOOS)
}

// Performs any required cleanup actions for the given container.
// Should only be called to revert any setup steps performed in setupNetworking.
func (m *cniNetworkManager) CleanupNetworking(_ context.Context, _ containerd.Container) error {
	return fmt.Errorf("CNI networking currently unsupported on %s", runtime.GOOS)
}

// Returns the set of NetworkingOptions which should be set as labels on the container.
func (m *cniNetworkManager) InternalNetworkingOptionLabels(_ context.Context) (types.NetworkOptions, error) {
	return m.netOpts, fmt.Errorf("CNI networking currently unsupported on %s", runtime.GOOS)
}

// Returns a slice of `oci.SpecOpts` and `containerd.NewContainerOpts` which represent
// the network specs which need to be applied to the container with the given ID.
func (m *cniNetworkManager) ContainerNetworkingOpts(_ context.Context, _ string) ([]oci.SpecOpts, []containerd.NewContainerOpts, error) {
	return []oci.SpecOpts{}, []containerd.NewContainerOpts{}, fmt.Errorf("CNI networking currently unsupported on %s", runtime.GOOS)
}
